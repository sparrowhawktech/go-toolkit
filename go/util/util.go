package util

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

var defaultStackTraceTag = "error"

func SetDefaultStackTraceTag(tag string) {
	defaultStackTraceTag = tag
}

var NewLine = func() string {
	switch runtime.GOOS {
	case "windows":
		return "\r\n"
	case "linux":
		return "\n"
	case "mac":
		return "\r"
	default:
		return "\n"
	}
}()

var startTime = time.Now()

func GetStartTime() time.Time {
	return startTime
}

type ErrorInfo struct {
	Message string
	Data    any
}

func BuildErrorInfo(message string, data any) ErrorInfo {
	return ErrorInfo{
		Message: message,
		Data:    data,
	}
}

func ThrowInfo(message string, data any) {
	panic(ErrorInfo{
		Message: message,
		Data:    data,
	})
}

type VersionInfo struct {
	Version *string
	Commit  *string
	Date    *string
}

var (
	versionInfo VersionInfo
)

func Write(w io.Writer, data []byte) {
	l, err := w.Write(data)
	CheckErr(err)
	if l != len(data) {
		panic(fmt.Sprintf("written length does not match: %d vs %d", l, len(data)))
	}
}

func Read(r io.Reader, buf []byte) {
	l, err := r.Read(buf)
	CheckErr(err)
	if l != len(buf) {
		panic(fmt.Sprintf("read length does not match: %d vs %d", l, len(buf)))
	}
}

func ReadUpTo(r io.Reader, buf []byte) int {
	l, err := r.Read(buf)
	CheckErr(err)
	return l
}

func WriteString(s string, w io.Writer) {
	_, err := io.WriteString(w, s)
	CheckErr(err)
}

func SeekHead(f *os.File, p int64) {
	offset, err := f.Seek(p, 0)
	CheckErr(err)
	if offset != p {
		panic(fmt.Sprintf("seek offset differs from expected: %d vs %d", p, offset))
	}
}

func SeekTail(f *os.File, p int64) {
	offset, err := f.Seek(p, 2)
	CheckErr(err)
	s, err := f.Stat()
	CheckErr(err)
	if offset != s.Size()-p {
		panic(fmt.Sprintf("seek offset differs from expected: %d vs %d", p, offset))
	}
}

func SetVersionInfo(tag string, date string, commit string) {
	versionInfo.Version = &tag
	versionInfo.Date = &date
	versionInfo.Commit = &commit
}

func GetVersionInfo() VersionInfo {
	return versionInfo
}

func FileExists(name string) bool {
	_, err := os.Stat(name)
	if err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false
	} else {
		panic(err)
	}
}

func Stat(fileSpec string) os.FileInfo {
	info, err := os.Stat(fileSpec)
	if err == nil {
		return info
	} else if os.IsNotExist(err) {
		return nil
	} else {
		panic(err)
	}
	return nil
}

func Caller(depth int) []string {
	result := make([]string, 0, depth)
	for i := 0; i < depth; i++ {
		pc, file, lineNo, ok := runtime.Caller(i + 2)
		if !ok {
			return result
		}
		funcName := runtime.FuncForPC(pc).Name()
		fileName := path.Base(file) // The Base function returns the last element of the path
		result = append(result, fmt.Sprintf("%s(%s:%d)", funcName, fileName, lineNo))
	}
	return result
}

func CheckErr(err interface{}) {
	if err != nil {
		panic(err)
	}
}

func CatchPanic() {
	if r := recover(); r != nil {
		ProcessError(r)
	}
}

func ProcessError(intf any, logTag ...string) {
	if len(logTag) > 0 {
		processErrorEx(intf, &logTag[0], nil)
	} else {
		processErrorEx(intf, nil, nil)
	}
}

func ResolveErrorMessage(e any) string {
	if err, ok := e.(error); ok {
		return err.Error()
	}
	v := reflect.ValueOf(e)
	k := v.Kind()
	if k == reflect.Ptr && v.IsZero() {
		return ""
	}
	if k == reflect.Ptr {
		v = v.Elem()
		k = v.Kind()
	}
	if IsStruct(v.Type()) {
		b, err := json.Marshal(e)
		if err != nil {
			r := fmt.Sprintf("%v", e)
			ProcessError(fmt.Sprintf("Could not marshal error %s: %v", r, err))
			return r
		} else {
			return string(b)
		}
	} else {
		return fmt.Sprintf("%v", e)
	}
}

var errorMutex = sync.Mutex{}
var errorMap = make(map[string]time.Time)

var noStackTraceTag = "-"

// Use with care. This will serialize and slow down your code. Make using it really worthy.
func ProcessErrorCompact(e any, category string, obsolescence time.Duration) {
	doProcessErrorCompact(e, category, obsolescence)
}

func doProcessErrorCompact(e any, category string, obsolescence time.Duration) {
	buffer := bytes.Buffer{}
	message := ResolveErrorMessage(e)
	buffer.WriteString(message)
	buffer.WriteString("@")
	buffer.WriteString(category)
	key := buffer.String()
	now := time.Now()
	if putError(key, now, obsolescence) {
		processErrorEx(e, nil, &defaultStackTraceTag)
	} else {
		processErrorEx(e, nil, &noStackTraceTag)
	}
}

func CatchPanicCompact(category string, obsolescence time.Duration) {
	if r := recover(); r != nil {
		doProcessErrorCompact(r, category, obsolescence)
	}
}

func putError(key string, now time.Time, obsolescence time.Duration) bool {
	errorMutex.Lock()
	defer errorMutex.Unlock()
	t0, ok := errorMap[key]
	if !ok || now.Sub(t0) > obsolescence {
		errorMap[key] = now
		return true
	} else {
		return false
	}
}

// sick and tired of not having the stack traces when I need them, banning this for now, removing soon
func processErrorEx(e any, logTag *string, stackTraceTag *string) {
	if e == nil {
		return
	}
	tag := "error"
	if logTag != nil {
		tag = *logTag
	}
	if stackTraceTag == nil {
		stackTraceTag = &defaultStackTraceTag
	}
	message := ResolveErrorMessage(e)
	if tag == *stackTraceTag || Loggable(*stackTraceTag) {
		stackTrace := string(debug.Stack())
		Log(*stackTraceTag).Printf("%s\n%s", message, stackTrace)
	} else if Loggable(*stackTraceTag) {
		stackTrace := string(debug.Stack())
		Log(*stackTraceTag).Printf("%s\n%s", message, stackTrace)
	} else {
		Log(tag).Printf("%s", message)
	}
}

func Unmarshal(bytes []byte, object any) {
	err := json.Unmarshal(bytes, object)
	if err != nil {
		panic(err)
	}
}

func Marshal(object any) []byte {
	jsonBytes, err := json.Marshal(object)
	CheckErr(err)
	return jsonBytes
}

func MarshalPretty(object interface{}) []byte {
	buffer := &bytes.Buffer{}
	JsonPretty(object, buffer)
	return buffer.Bytes()
}

func TruncDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func LoadConfig(path string, config interface{}) {
	abs, err := filepath.Abs(path)
	CheckErr(err)
	data, err := os.ReadFile(abs)
	CheckErr(err)
	err = json.Unmarshal(data, config)
	CheckErr(err)
}

func SaveConfig(path string, config interface{}) {
	abs, err := filepath.Abs(path)
	CheckErr(err)
	file, err := os.Create(abs)
	defer file.Close()
	CheckErr(err)
	JsonPretty(config, file)
}

func ParseInt(s string) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	CheckErr(err)
	return i
}

func ParseBool(s string) bool {
	b, err := strconv.ParseBool(s)
	CheckErr(err)
	return b
}

func JsonDecode(i interface{}, r io.Reader) interface{} {
	err := json.NewDecoder(r).Decode(i)
	CheckErr(err)
	return i
}

func JsonEncode(i interface{}, w io.Writer) {
	err := json.NewEncoder(w).Encode(i)
	CheckErr(err)
}

func JsonPretty(i interface{}, w io.Writer) {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "    ")
	err := encoder.Encode(i)
	CheckErr(err)
	_, err = w.Write([]byte("\n"))
	CheckErr(err)
}

func Ptr[T any](v T) *T {
	return &v
}

func PStr(s string) *string {
	return &s
}

func PStrf(s string, values ...interface{}) *string {
	r := fmt.Sprintf(s, values...)
	return &r
}

func PInt64(i int64) *int64 {
	return &i
}

func PUint16(i uint16) *uint16 {
	return &i
}

func PUint32(i uint32) *uint32 {
	return &i
}

func PUint64(i uint64) *uint64 {
	return &i
}

func PInt(i int) *int {
	return &i
}

func PInt32(i int32) *int32 {
	return &i
}

func PFloat32(f float32) *float32 {
	return &f
}

func PFloat64(f float64) *float64 {
	return &f
}

func PTime(t time.Time) *time.Time {
	return &t
}

func PBool(b bool) *bool {
	return &b
}

func PJson(b []byte) *json.RawMessage {
	rm := json.RawMessage(b)
	return &rm
}

func Help(o interface{}) string {
	buffer := &bytes.Buffer{}
	thisType := reflect.TypeOf(o)
	elem := thisType.Elem()
	for i := 0; i < elem.NumField(); i++ {
		f := elem.Field(i)
		s := f.Tag.Get("help")
		if s != "" {
			buffer.WriteString(f.Tag.Get("json"))
			buffer.WriteString(": ")
			buffer.WriteString(s)
			buffer.WriteString("\r\n")
		}
	}
	return buffer.String()
}

/*
*
XPath-ish deep-graph search for []interface{}/map[string]interface{} (generic json decode)
Use .<field name> for deep-graph navigation, #<index> for array position
Example:
Given m := { "a" : {"a-1" : {"a-1-list":[1, 2, 3]}}}
XFind(m, "a.a-1.a-1-list#1") will return 2
*/
func XFind(data interface{}, path string) interface{} {
	steps := strings.Split(path, ".")
	current := data
	for _, key := range steps {
		if strings.HasPrefix(key, "#") {
			list := current.([]interface{})
			index := int(ParseInt(key[1:]))
			if index >= len(list) {
				return nil
			} else {
				current = list[index]
			}
		} else {
			object := current.(map[string]interface{})
			value, ok := object[key]
			if ok {
				current = value
			} else {
				return nil
			}
		}
	}
	return current
}

func XRetrieve(data interface{}, path string) interface{} {
	value := XFind(data, path)
	if value == nil {
		panic(fmt.Sprintf("No value found for %s", path))
	}
	return value
}

func XFindString(data interface{}, path string) *string {
	r := XFind(data, path)
	if r == nil {
		return nil
	} else {
		switch v := r.(type) {
		case int:
			str := fmt.Sprintf("%d", v)
			return &str
		case float64:
			str := fmt.Sprintf("%f", v)
			return &str
		default:
			return PStr(v.(string))
		}
	}
}

func IsHostIP(hostIp net.IP) bool {
	ifaces, err := net.Interfaces()
	CheckErr(err)
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		CheckErr(err)
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if bytes.Equal(hostIp, ip) {
				return true
			}
		}
	}
	return false
}

func ParseUnixTimestamp(unixTimestamp uint64) time.Time {
	oneSecond := float64(1000000000)
	seconds := float64(unixTimestamp) / float64(oneSecond)
	intSecs := int64(seconds)
	delta := seconds - float64(intSecs)
	nanos := int64(delta * oneSecond)
	return time.Unix(int64(seconds), nanos)
}

func RunCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed executing %s with error %v\nCombined output:\n%s\n", cmd.String(), err, string(out)))
	}
	return string(out)
}

func SafeRunCmd(cmd string, args ...string) *string {
	defer CatchPanic()
	result := RunCmd(cmd, args...)
	return &result
}

func RunCmdTo(w io.Writer, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = w
	b := bytes.Buffer{}
	cmd.Stderr = &b
	err := cmd.Start()
	CheckErr(err)
	err = cmd.Wait()
	if err != nil {
		panic(fmt.Sprintf("Failed executing %s with error %v\nCombined output:\n%s\n", cmd.String(), err, b.String()))
	}
}

func RunCmdGrep(command string, grepChain ...string) string {
	cmd := exec.Command("bash", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed executing [%s] with error [%v]\n%s", cmd.String(), err, string(out)))
	}

	return grep(out, grepChain, 0)
}

func grep(input []byte, chain []string, chainIndex int) string {
	token := chain[chainIndex]
	inputBuffer := bytes.NewBuffer(input)
	outputBuffer := &bytes.Buffer{}
	l, err := inputBuffer.ReadString('\n')
	for err != io.EOF {
		if err != nil {
			panic(err)
		}
		if strings.Contains(l, token) {
			outputBuffer.WriteString(l)
			outputBuffer.WriteByte('\n')
		}
		l, err = inputBuffer.ReadString('\n')
	}
	chainIndex++
	if chainIndex < len(chain) {
		return grep(outputBuffer.Bytes(), chain, chainIndex)
	} else {
		return outputBuffer.String()
	}
}

func WaitFor[T any](ch chan T, d time.Duration, message string) T {
	select {
	case result := <-ch:
		Log("info").Printf("%s: %s", message, ResolveErrorMessage(result))
		return result
	case <-time.After(d):
		panic(fmt.Sprintf("Timed out waiting for %s", message))
	}
}

func Encrypt(data []byte, key []byte) []byte {
	block, err := aes.NewCipher(key)
	CheckErr(err)

	gcm, err := cipher.NewGCM(block)
	CheckErr(err)

	nonce := make([]byte, gcm.NonceSize())
	_, err = rand.Read(nonce)
	CheckErr(err)

	return gcm.Seal(nonce, nonce, data, nil)
}

func Decrypt(data []byte, key []byte) []byte {
	block, err := aes.NewCipher(key)
	CheckErr(err)

	gcm, err := cipher.NewGCM(block)
	CheckErr(err)

	nonceSize := gcm.NonceSize()

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]

	result, err := gcm.Open(nil, nonce, ciphertext, nil)
	CheckErr(err)
	return result
}

func Close(c io.Closer) {
	err := c.Close()
	CheckErr(err)
}

func SafeClose(c io.Closer) {
	err := c.Close()
	if err != nil {
		ProcessError(err, "error")
	}
}

func RemoveFileSafe(spec string) {
	defer CatchPanic()
	RemoveFile(spec)
}

func RemoveFile(spec string) {
	err := os.Remove(spec)
	CheckErr(err)
}

func NilOrValue(p interface{}) interface{} {
	v := reflect.ValueOf(p)
	if v.Kind() != reflect.Ptr {
		return p
	} else if v.IsNil() || v.IsZero() {
		return nil
	} else {
		return v.Elem().Interface()
	}
}

func CalculateSHA256(filePath string) string {
	file, err := os.Open(filePath)
	CheckErr(err)
	defer file.Close()
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	CheckErr(err)
	hashInBytes := hash.Sum(nil)
	hashString := hex.EncodeToString(hashInBytes)

	return hashString
}

func OpenFile(spec string) *os.File {
	f, err := os.Open(spec)
	CheckErr(err)
	return f
}

func CreateFile(spec string) *os.File {
	f, err := os.Create(spec)
	CheckErr(err)
	return f
}

type FriendlyErrorMessage struct {
	ErrorMessage string `json:"errorMessage"`
}

func ThrowFriendly(message string, args ...any) {
	panic(FriendlyErrorMessage{
		ErrorMessage: fmt.Sprintf(message, args...),
	})
}
