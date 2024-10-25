package util

import "flag"

func FlagString(name string, value string, usage string) *string {
	var p string
	if flag.Lookup(name) == nil {
		flag.StringVar(&p, name, value, usage)
	}
	return &p
}

func FlagInt(name string, value int, usage string) *int {
	var p int
	if flag.Lookup(name) == nil {
		flag.IntVar(&p, name, value, usage)
	}
	return &p
}

func FlagInt64(name string, value int64, usage string) *int64 {
	var p int64
	if flag.Lookup(name) == nil {
		flag.Int64Var(&p, name, value, usage)
	}
	return &p
}

func FlagBool(name string, value bool, usage string) *bool {
	var p bool
	if flag.Lookup(name) == nil {
		flag.BoolVar(&p, name, value, usage)
	}
	return &p
}
