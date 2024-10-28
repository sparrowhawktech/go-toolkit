package web

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"sparrowhawktech/toolkit/auth"
	"sparrowhawktech/toolkit/sql"
	"sparrowhawktech/toolkit/tx"
	"sparrowhawktech/toolkit/util"
)

func InterceptCORS(delegate func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			header := r.Header.Get("Access-Control-Request-Headers")
			if len(header) > 0 {
				w.Header().Add("Access-Control-Allow-Headers", header)
			}
		} else {
			delegate(w, r)
		}
	}
}

func resolveSecret(r *http.Request) *string {
	c, err := r.Cookie("secret")
	if err == nil {
		return &c.Value
	}
	value := r.Header.Get("secret")
	if len(value) > 0 {
		return &value
	}
	values, ok := r.URL.Query()["secret"]
	if ok {
		return &values[0]
	}
	return nil
}

func InterceptSecret(secret string, delegate func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		incoming := resolveSecret(r)
		if secret != "" {
			if incoming == nil {
				panic("Unauthorized operation")
			}
			if *incoming != secret {
				panic("Invalid credentials")
			}
		}
		delegate(w, r)
	}
}

func InterceptFatal(delegate func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return InterceptStats(func(w http.ResponseWriter, r *http.Request) {
		defer catchFatal(w, r)
		defer func() {
			err := r.Body.Close()
			if err != nil {
				util.Log("error").Printf("Could not close response body: %v", err)
			}
		}()
		delegate(w, r)
	})
}

func InterceptStats(delegate func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t0 := time.Now()
		stats.PushIn(r.URL.Path)
		defer func() {
			t1 := time.Now()
			d := t1.Sub(t0).Milliseconds()
			stats.PushOut(r.URL.Path, d)
		}()
		delegate(w, r)
	}
}

func catchFatal(writer http.ResponseWriter, r *http.Request) {
	if e := recover(); e != nil {
		util.ProcessError(e, "error")
		writer.WriteHeader(http.StatusInternalServerError)
		if err, ok := e.(util.FriendlyErrorMessage); ok {
			writer.Header().Set(HeaderContentType, ContentTypeApplicationJson)
			writer.Header().Set(ErrorHeaderName, "true")
			util.JsonEncode(err, writer)
		} else if err, ok := e.(FriendlyErrorResponse); ok {
			writer.Header().Set(HeaderContentType, ContentTypeApplicationJson)
			writer.Header().Set(ErrorHeaderName, "true")
			util.JsonEncode(err, writer)
		} else if reflect.TypeOf(r).Kind() == reflect.Struct {
			isJson, jsonBytes := marshalError(r)
			if isJson {
				writer.Header().Set(HeaderContentType, ContentTypeApplicationJson)
			} else {
				writer.Header().Set(HeaderContentType, "text/plain")
			}
			_, err := writer.Write(jsonBytes)
			if err != nil {
				util.ProcessError(err)
			}
		} else {
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			writer.Write([]byte(fmt.Sprint(e)))
		}
	}
}

func marshalError(e interface{}) (isJson bool, result []byte) {
	defer func() {
		isJson = false
		result = []byte(fmt.Sprintf("%v", e))
	}()
	return true, util.Marshal(e)
}

func ConfigureHandlerTransactional(serveMux *http.ServeMux, path string, datasourceConfig sql.DatasourceConfig, f func(txContext *tx.Transaction, w http.ResponseWriter, r *http.Request)) {
	if serveMux == nil {
		serveMux = http.DefaultServeMux
	}
	serveMux.HandleFunc(path, InterceptFatal(InterceptCORS(tx.InterceptTransactional(datasourceConfig, InterceptAudit(f)))))
}

func ConfigureHandlerAuthenticated(serveMux *http.ServeMux, path string, sessionManager *auth.SessionManager, f func(w http.ResponseWriter, r *http.Request)) {
	serveMux.HandleFunc(path, InterceptFatal(InterceptCORS(InterceptAuth(sessionManager, f))))
}

func ConfigureHandlerAuthenticatedTransactional(serveMux *http.ServeMux, path string, sessionManager *auth.SessionManager, databaseConfig sql.DatasourceConfig, f func(trx *tx.Transaction, w http.ResponseWriter, r *http.Request)) {
	serveMux.HandleFunc(path, InterceptFatal(InterceptCORS(InterceptAuth(sessionManager, tx.InterceptTransactional(databaseConfig, InterceptAudit(f))))))
}

func InterceptAudit(delegate func(tx *tx.Transaction, w http.ResponseWriter, r *http.Request)) func(tx *tx.Transaction, w http.ResponseWriter, r *http.Request) {
	return func(tx *tx.Transaction, w http.ResponseWriter, r *http.Request) {
		sessionEntry := r.Context().Value("sessionEntry")
		if sessionEntry != nil {
			tx.Exec(fmt.Sprintf("set local audit.user_name to '%d';", *sessionEntry.(*auth.SessionEntry).UserId))
		}
		tx.Exec(fmt.Sprintf("set local audit.context to '%s';", r.URL.Path))
		delegate(tx, w, r)
	}
}

func InterceptAuth(sessionManager *auth.SessionManager, delegate func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenEntry := validateToken(r, sessionManager)
		if tokenEntry == nil {
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			ctx := context.WithValue(r.Context(), "sessionEntry", tokenEntry)
			delegate(w, r.WithContext(ctx))
		}
	}
}

func validateToken(r *http.Request, sessionManager *auth.SessionManager) *auth.SessionEntry {
	token, ok := resolveToken(r)
	if ok {
		return sessionManager.ValidateToken(token)
	} else {
		return nil
	}
}

func resolveToken(r *http.Request) (string, bool) {
	value := r.Header.Get("Toolkit-Authorization")
	if len(value) > 0 {
		return value, true
	}
	value = r.Header.Get("Authorization")
	if len(value) > 0 {
		parts := strings.Split(value, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			return parts[1], true
		}
	}
	values, ok := r.URL.Query()["toolkitToken"]
	if ok {
		return values[0], true
	}
	return "", false
}

func InterceptSigned(secret string, delegate func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientId := r.Header.Get(ClientIdHeaderName)
		timestamp := r.Header.Get(TimestampHeaderName)
		signature := r.Header.Get(SignatureHeaderName)
		computed := CreateSignature(secret, []byte(clientId+"."+timestamp))

		if strings.Compare(signature, computed) != 0 {
			panic("Invalid signature")
		}
		ctx := context.WithValue(r.Context(), "clientId", clientId)
		delegate(w, r.WithContext(ctx))
	}
}

func InterceptBasicAuth(delegate func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		inUsername, inPw, ok := r.BasicAuth()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			ctx := context.WithValue(r.Context(), "username", inUsername)
			ctx = context.WithValue(ctx, "password", inPw)
			delegate(w, r.WithContext(ctx))
		}
	}
}
