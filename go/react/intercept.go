package react

import (
	"io/ioutil"
	"net/http"

	"sparrowhawktech/toolkit/util"
)

type ResponseWriter struct {
	http.ResponseWriter
	notFound bool
}

func (o *ResponseWriter) WriteHeader(status int) {
	o.notFound = status == 404
	if !o.notFound {
		o.ResponseWriter.WriteHeader(status)
	}
}

func (o *ResponseWriter) Write(b []byte) (int, error) {
	if o.notFound {
		return 0, nil
	} else {
		return o.ResponseWriter.Write(b)
	}
}

func InterceptReact(folder string, h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		interceptor := ResponseWriter{ResponseWriter: w, notFound: false}
		h.ServeHTTP(&interceptor, r)
		if interceptor.notFound {
			content, err := ioutil.ReadFile(folder + "/index.html")
			if err == nil {
				w.Header().Set("Content-Type", "text/html")
				_, err := w.Write(content)
				util.CheckErr(err)
			}
		}
	}
}
