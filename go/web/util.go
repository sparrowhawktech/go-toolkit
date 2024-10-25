package web

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"sparrowhawktech/toolkit/react"
	"sparrowhawktech/toolkit/sql"
	"sparrowhawktech/toolkit/tx"
	"sparrowhawktech/toolkit/util"
)

const (
	HeaderContentType          = "Content-Type"
	HeaderAuthorization        = "Authorization"
	ContentTypeApplicationJson = "application/json"
	ContentTypeOctetStream     = "octet/stream"
	ClientIdHeaderName         = "Toolkit-ClientId"
	TimestampHeaderName        = "Toolkit-Timestamp"
	SignatureHeaderName        = "Toolkit-Signature"
	ErrorHeaderName            = "Toolkit-Error"
)

type FriendlyErrorResponse = util.FriendlyErrorMessage

type DictionaryErrorResponse struct {
	ErrorCode    int         `json:"errorCode"`
	ErrorMessage string      `json:"errorMessage"`
	Data         interface{} `json:"data"`
}

type HttpErrorResponse struct {
	StatusCode int
	Body       string
	Url        string
}

func (o HttpErrorResponse) String() string {
	return string(util.Marshal(o))
}

func JsonResponse(i interface{}, w http.ResponseWriter) {
	w.Header().Set(HeaderContentType, ContentTypeApplicationJson)
	util.JsonEncode(i, w)
}

func JsonResponsePretty(i interface{}, w http.ResponseWriter) {
	w.Header().Set(HeaderContentType, ContentTypeApplicationJson)
	util.JsonPretty(i, w)
}

func interceptDebug(delegate func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if util.Loggable("debug") {
			util.Log("debug").Printf("Handling %s from %s", r.URL.String(), r.RemoteAddr)
		}
		delegate(w, r)
	}
}

func HandleDefault(serveMux *http.ServeMux, path string, f func(w http.ResponseWriter, r *http.Request)) {
	if serveMux == nil {
		serveMux = http.DefaultServeMux
	}
	serveMux.HandleFunc(path, InterceptFatal(InterceptCORS(f)))
}

func HandleBasicAuth(serveMux *http.ServeMux, path string, f func(w http.ResponseWriter, r *http.Request)) {
	if serveMux == nil {
		serveMux = http.DefaultServeMux
	}
	serveMux.HandleFunc(path, InterceptFatal(InterceptBasicAuth(InterceptCORS(f))))
}

func ConfigureHandlerSecret(serveMux *http.ServeMux, path string, secret string, f func(w http.ResponseWriter, r *http.Request)) {
	if serveMux == nil {
		serveMux = http.DefaultServeMux
	}
	serveMux.HandleFunc(path, InterceptFatal(InterceptSecret(secret, InterceptCORS(f))))
}

func ConfigureHandlerWithDebug(serveMux *http.ServeMux, path string, f func(w http.ResponseWriter, r *http.Request)) {
	if serveMux == nil {
		serveMux = http.DefaultServeMux
	}
	serveMux.HandleFunc(path, InterceptFatal(interceptDebug(InterceptCORS(f))))
}

func HandleUi(mux *http.ServeMux, name string, path string) {
	var folder string
	if util.FileExists("ui/" + name) {
		// Production
		folder = "ui/" + name
	} else if util.FileExists("../../ui/" + name) {
		// Simulate Release Package
		folder = "../../ui/" + name
	} else {
		// Dev
		folder = "../../ui/build"
	}
	abs, _ := filepath.Abs(folder)
	util.Log("info").Printf("Publishing ui folder: %s", abs)
	fs := http.FileServer(http.Dir(abs))
	sp := http.StripPrefix(path, fs)
	mux.Handle(path, InterceptFatal(InterceptCORS(react.InterceptReact(folder, sp))))
}

func CheckResponse(r *http.Response, greaterThan int) {
	if r.StatusCode > greaterThan {
		if r.StatusCode == 500 && r.Header.Get(ErrorHeaderName) == "true" {
			errorResponse := FriendlyErrorResponse{}
			util.JsonDecode(&errorResponse, r.Body)
			panic(errorResponse)
		} else {
			data, err := io.ReadAll(r.Body)
			util.CheckErr(err)
			panic(HttpErrorResponse{Url: r.Request.URL.String(), StatusCode: r.StatusCode, Body: string(data)})
		}
	}
}

func IsJsonContentType(r *http.Response) bool {
	return r.Header.Get(HeaderContentType) == ContentTypeApplicationJson
}

func CloseResponse(response *http.Response) {
	err := response.Body.Close()
	if err != nil {
		util.ProcessError(err)
	}
}

func ResolveErrorMessage(r interface{}, defaultMessage string) string {
	if err, ok := r.(*url.Error); ok {
		return fmt.Sprintf("Url Error: %s\n", err.Error())
	} else if err, ok := r.(FriendlyErrorResponse); ok {
		return err.ErrorMessage
	} else if err, ok := r.(util.FriendlyErrorMessage); ok {
		return err.ErrorMessage
	} else {
		return fmt.Sprintf("Error: %s\n%v.\n", defaultMessage, r)
	}
}

func CatchFriendlyAndExit(defaultMessage string) {
	if r := recover(); r != nil {
		message := ResolveErrorMessage(r, defaultMessage)
		fmt.Println("\n" + message)
		os.Exit(1)
	}
}

func ListenAndWait(serveMux *http.ServeMux, port int) *http.Server {
	localAddress := fmt.Sprintf(":%d", port)
	util.Log("info").Println("Starting http server at " + localAddress)
	httpServer := &http.Server{Addr: localAddress, Handler: serveMux}
	go func() {
		err := httpServer.ListenAndServe()
		if err != http.ErrServerClosed {
			util.CheckErr(err)
		}
	}()

	client := &http.Client{}
	n := 0
	for {
		if n > 10 {
			panic("Mock services http server is not responding")
		}
		_, err := client.Get(fmt.Sprintf("http://localhost:%d/ping", port))
		if err == nil {
			break
		} else {
			util.Log("warning").Printf("%v. Retrying...", err)
			n++
			time.Sleep(time.Millisecond * 500)
		}
	}
	return httpServer
}

func JsonRequest(method string, url string, out interface{}, in interface{}, timeout time.Duration, maxResult int) {
	buffer := &bytes.Buffer{}
	if out != nil {
		util.JsonEncode(out, buffer)
	}
	request, err := http.NewRequest(method, url, buffer)
	util.CheckErr(err)
	request.Header.Set(HeaderContentType, ContentTypeApplicationJson)
	config := &tls.Config{}
	transport := &http.Transport{TLSClientConfig: config}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
	response, err := client.Do(request)
	util.CheckErr(err)
	defer CloseResponse(response)
	CheckResponse(response, maxResult)
	if in != nil {
		util.JsonDecode(in, response.Body)
	}
}

func GetJson(url string, timeout time.Duration, maxResult int, headers map[string]string, entity interface{}) {
	response := requestGet(url, timeout, headers)
	defer CloseResponse(response)
	CheckResponse(response, maxResult)
	util.JsonDecode(entity, response.Body)
}

func Get(url string, timeout time.Duration, maxResult int, headers map[string]string) []byte {
	response := requestGet(url, timeout, headers)
	defer CloseResponse(response)
	CheckResponse(response, maxResult)
	result := &bytes.Buffer{}
	_, err := io.Copy(result, response.Body)
	util.CheckErr(err)
	return result.Bytes()
}

func GetStream(url string, timeout time.Duration, maxResult int, headers map[string]string, w io.Writer) {
	response := requestGet(url, timeout, headers)
	defer CloseResponse(response)
	CheckResponse(response, maxResult)
	_, err := io.Copy(w, response.Body)
	util.CheckErr(err)
}

func requestGet(url string, timeout time.Duration, headers map[string]string) *http.Response {
	buffer := &bytes.Buffer{}
	request, err := http.NewRequest("GET", url, buffer)
	util.CheckErr(err)
	for k, v := range headers {
		request.Header.Add(k, v)
	}
	config := &tls.Config{}
	transport := &http.Transport{TLSClientConfig: config}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
	response, err := client.Do(request)
	util.CheckErr(err)
	return response
}

func CreateSignature(secret string, data []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(data)
	encrypted := h.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(encrypted)
}

func ConfigureHandlerSigned(serveMux *http.ServeMux, path string, secret string, f func(w http.ResponseWriter, r *http.Request)) {
	serveMux.HandleFunc(path, InterceptFatal(InterceptCORS(InterceptSigned(secret, f))))
}

func ConfigureHandlerSignedTransactional(serveMux *http.ServeMux, path string, secret string, databaseConfig sql.DatasourceConfig, f func(trx *tx.Transaction, w http.ResponseWriter, r *http.Request)) {
	serveMux.HandleFunc(path, InterceptFatal(InterceptCORS(InterceptSigned(secret, tx.InterceptTransactional(databaseConfig, f)))))
}

func PostJson(url string, out interface{}, in interface{}, maxResult int, timeout time.Duration, headers map[string]string) {
	buffer := &bytes.Buffer{}
	if out != nil {
		util.JsonEncode(out, buffer)
	}
	request, err := http.NewRequest("POST", url, buffer)
	util.CheckErr(err)
	request.Close = true
	request.Header.Set(HeaderContentType, "application/json")
	for k, v := range headers {
		request.Header.Set(k, v)
	}

	client := &http.Client{
		Transport: &http.Transport{},
		Timeout:   timeout,
	}
	response, err := client.Do(request)
	util.CheckErr(err)
	defer CloseResponse(response)
	CheckResponse(response, maxResult)
	if in != nil {
		util.JsonDecode(in, response.Body)
	}
}

func PostJsonSigned(url string, out interface{}, in interface{}, maxResult int, timeout time.Duration, clientId string, clientSecret string) {
	timestamp := time.Now().Format(time.RFC3339)
	signature := CreateSignature(clientSecret, []byte(clientId+"."+timestamp))
	PostJson(url, out, in, maxResult, timeout, map[string]string{ClientIdHeaderName: clientId, TimestampHeaderName: timestamp, SignatureHeaderName: signature})
}

func Request(method string, url string, out io.Reader, in io.Writer, maxResult int, timeout time.Duration, headers map[string]string) {
	request, err := http.NewRequest(method, url, out)
	util.CheckErr(err)
	request.Close = true
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	client := &http.Client{
		Timeout: timeout,
	}
	response, err := client.Do(request)
	util.CheckErr(err)
	defer CloseResponse(response)
	CheckResponse(response, maxResult)
	if in != nil {
		_, err = io.Copy(in, response.Body)
		util.CheckErr(err)
	}
}

// ValidateStruct Assumes all members are pointers and recursivly evaluates assigment only and only if
// the tag "require" is present
// and the tag value is "true"
// If the tag's value is true and the member value is nil, then panics
func ValidateStruct(s interface{}) {
	doValidateStruct(s, "")
}

func doValidateStruct(s interface{}, path string) {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fd := t.Field(i)
		if fd.IsExported() {
			f := v.Field(i)
			tv, ok := fd.Tag.Lookup("require")
			if ok && tv == "true" && f.IsNil() {
				panic(fmt.Sprintf("Required field missing: %s.%s", path, fd.Name))
			}
			if util.IsStructPtr(f) {
				doValidateStruct(f.Elem().Interface(), path+"."+fd.Name)
			}
		}
	}
}

func Listen(httpServer *http.Server) {
	err := httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}
