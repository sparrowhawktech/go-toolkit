package coverage

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"time"

	"sparrowhawktech/toolkit/util"
	"sparrowhawktech/toolkit/web"
)

type HttpClient struct {
	headers  map[string]interface{}
	serveMux *http.ServeMux
}

func (o *HttpClient) SetHeader(header string, value string) {
	o.headers[header] = value
}

func (o *HttpClient) Post(url string, payload interface{}, result interface{}) {
	res := o.postJson(url, payload)
	CheckStatusResponse(res)
	if result != nil {
		result = util.JsonDecode(result, res.Body)
	}
}

func (o *HttpClient) PostForm(url string, payload *bytes.Buffer, result interface{}, contentType string) {

	req, err := http.NewRequest("POST", url, payload)
	req.Header.Set(web.HeaderContentType, contentType)
	util.CheckErr(err)

	res := o.sendRequest(req)
	CheckStatusResponse(res)
	if result != nil {
		util.JsonDecode(result, res.Body)
	}
}

func (o *HttpClient) PostRaw(url string, payload interface{}) *httptest.ResponseRecorder {
	return o.postJson(url, payload)
}

func CheckStatusResponse(resp *httptest.ResponseRecorder) {
	if resp.Code != http.StatusOK {
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("Error reading response payload: %v", err)
		}
		panic(fmt.Sprintf("HTTP ERROR %d\n%s", resp.Code, string(data)))
	}
}

func (o *HttpClient) postJson(url string, payload interface{}) *httptest.ResponseRecorder {
	b := bytes.NewBuffer([]byte{})
	util.JsonEncode(payload, b)
	req, err := http.NewRequest("POST", url, b)
	util.CheckErr(err)
	req.Header.Set(web.HeaderContentType, "application/json")
	return o.sendRequest(req)
}

func (o *HttpClient) Get(url string, result interface{}) {
	b := bytes.NewBuffer([]byte{})
	req, err := http.NewRequest("GET", url, b)
	util.CheckErr(err)
	req.Header.Set(web.HeaderContentType, "application/json")
	response := o.sendRequest(req)
	CheckStatusResponse(response)
	if result != nil {
		result = util.JsonDecode(result, response.Body)
	}
}

func (o *HttpClient) sendRequest(req *http.Request) *httptest.ResponseRecorder {
	if o.headers != nil {
		for header, value := range o.headers {
			req.Header.Set(header, value.(string))
		}
	}
	rr := httptest.NewRecorder()
	o.serveMux.ServeHTTP(rr, req)
	return rr
}

func NewHttpClient(serveMux *http.ServeMux) *HttpClient {
	headers := make(map[string]interface{})
	return &HttpClient{
		headers:  headers,
		serveMux: serveMux,
	}
}

func StartHttpServer(serveMux *http.ServeMux, httpPort int) *http.Server {
	serveMux.HandleFunc("/coverage/ping", func(writer http.ResponseWriter, request *http.Request) {

	})
	localAddress := fmt.Sprintf(":%d", httpPort)
	util.Log("info").Println("Starting http server at " + localAddress)
	httpServer := &http.Server{Addr: localAddress, Handler: serveMux}
	go func() {
		httpServer.SetKeepAlivesEnabled(false)
		err := httpServer.ListenAndServe()
		if err != http.ErrServerClosed {
			util.CheckErr(err)
		}
	}()

	probe := func() (result bool) {
		result = false
		defer util.CatchPanic()
		web.Get(fmt.Sprintf("http://localhost:%d/coverage/ping", httpPort), time.Second, 200, nil)
		return true
	}

	n := 0
	for {
		if probe() {
			break
		}
		time.Sleep(time.Millisecond * 500)
		n++
		if n == 3 {
			panic("Http server warm up probe failed")
		}
	}

	return httpServer
}
