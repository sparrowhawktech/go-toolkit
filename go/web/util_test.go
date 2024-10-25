package web_test

import (
	"fmt"
	"sparrowhawktech/toolkit/util"
	"sparrowhawktech/toolkit/web"
	"testing"
)

func TestValidateStruct(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			panic("Required fields not validated")
		} else {
			fmt.Printf("%v\n", r)
		}
	}()
	type S1 struct {
		A *string
	}
	type S2 struct {
		A   *string
		B   *string
		C   *string `require:"false"`
		D   *int
		S1A *S1
		S1B *S1
	}
	s2 := S2{
		A: nil,
		B: util.PStr(""),
		C: nil,
		S1A: &S1{
			A: util.PStr("hi mom"),
		},
		S1B: nil,
	}
	web.ValidateStruct(s2)
}
