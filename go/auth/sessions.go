package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"sparrowhawktech/toolkit/util"
)

type DataProvider interface {
	LoadSnapshot() map[string]*SessionEntry
	CreateSession(entry *SessionEntry) int64
	UpdateSessionTime(id int64, expirationTime time.Time, lastTime time.Time)
	RemoveSession(entry *SessionEntry)
	Shrink()
}

type SessionsConfig struct {
	Secret       *string `json:"secret"`
	TokenTimeout *int    `json:"tokenTimeout"`
}

type JwtTokenHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type JwtTokenPayload struct {
	UserId         int64     `json:"userId"`
	MinutesTimeout int       `json:"minutesTimeout"`
	CreationTime   time.Time `json:"creationTime"`
}

type SessionEntry struct {
	UserId         *int64
	CreationTime   *time.Time
	ExpirationTime *time.Time
	LastTime       *time.Time
	TokenString    *string
	Id             *int64
}

type SessionManager struct {
	DataProvider DataProvider
	JwtConfig    SessionsConfig
	SessionMap   map[string]*SessionEntry
	Mux          sync.Mutex
}

func (o *SessionsConfig) Validate() {
	if o.Secret == nil {
		panic("Invalid secret")
	}
	if o.TokenTimeout == nil {
		panic("Invalid tokenTimeout")
	}
}

func (o *SessionManager) EvictToken(tokenString string) {
	entry := o.doEvictToken(tokenString)
	o.DataProvider.RemoveSession(entry)
}

func (o *SessionManager) doEvictToken(value string) *SessionEntry {
	o.Mux.Lock()
	defer o.Mux.Unlock()
	entry := o.SessionMap[value]
	delete(o.SessionMap, value)
	return entry
}

func (o *SessionManager) CreateToken(userId int64) string {

	header := JwtTokenHeader{Alg: "HS256", Typ: "JWT"}

	payload := JwtTokenPayload{UserId: userId, MinutesTimeout: *o.JwtConfig.TokenTimeout, CreationTime: time.Now()}

	b, err := json.Marshal(header)
	util.CheckErr(err)
	content1 := base64.RawURLEncoding.EncodeToString(b)

	b, err = json.Marshal(payload)
	util.CheckErr(err)
	content2 := base64.RawURLEncoding.EncodeToString(b)

	content := content1 + "." + content2

	keyString := *o.JwtConfig.Secret
	key := []byte(keyString)
	h := hmac.New(sha256.New, key)
	_, err = h.Write([]byte(content))
	util.CheckErr(err)
	signature := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	token := fmt.Sprintf("%s.%s", content, signature)
	tokenEntry := o.registerToken(&payload, token)
	id := o.DataProvider.CreateSession(tokenEntry)
	tokenEntry.Id = &id
	return token
}

func (o *SessionManager) ValidateToken(token string) *SessionEntry {
	o.Mux.Lock()
	defer o.Mux.Unlock()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		panic("Invalid token")
	}
	payload := JwtTokenPayload{}
	decodeTokenPart(&payload, parts[1])
	entry, ok := o.SessionMap[token]
	if !ok {
		return nil
	}
	if entry.ExpirationTime.Before(time.Now()) {
		delete(o.SessionMap, token)
		return nil
	}
	if *entry.UserId != payload.UserId {
		return nil
	}
	now := time.Now()
	entry.LastTime = &now
	entry.ExpirationTime = util.PTime(now.Add(time.Minute * time.Duration(*o.JwtConfig.TokenTimeout)))
	tokenCopy := *entry
	return &tokenCopy
}

func (o *SessionManager) registerToken(payload *JwtTokenPayload, token string) *SessionEntry {
	o.Mux.Lock()
	defer o.Mux.Unlock()
	expiration := time.Now().Add(time.Minute * time.Duration(*o.JwtConfig.TokenTimeout))
	now := time.Now()
	te := SessionEntry{UserId: &payload.UserId, CreationTime: &now, ExpirationTime: &expiration, LastTime: &now, TokenString: &token}
	o.SessionMap[token] = &te
	return &te
}

func (o *SessionManager) Shrink() {
	o.DataProvider.Shrink()
}

func (o *SessionManager) Load() {
	o.SessionMap = o.DataProvider.LoadSnapshot()
}

func NewSessionManager(dataProvider DataProvider, jwtConfig SessionsConfig) *SessionManager {
	tm := SessionManager{DataProvider: dataProvider, JwtConfig: jwtConfig, SessionMap: make(map[string]*SessionEntry), Mux: sync.Mutex{}}
	return &tm
}

func decodeTokenPart(i interface{}, part string) {
	jsonBytes, err := base64.RawURLEncoding.DecodeString(part)
	util.CheckErr(err)
	util.JsonDecode(i, bytes.NewReader(jsonBytes))
}
