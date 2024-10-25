package web

import (
	"encoding/json"
	"net/http"
	"sparrowhawktech/toolkit/util"
	"strings"
)

type HttpError struct {
	StatusCode int
	Error      interface{}
}

func ParseParamOrBody(r *http.Request, o interface{}) {
	s := r.URL.Query().Get("body")
	if len(s) > 0 {
		util.CheckErr(json.NewDecoder(strings.NewReader(s)).Decode(o))
	} else {
		util.CheckErr(json.NewDecoder(r.Body).Decode(o))
	}
}
