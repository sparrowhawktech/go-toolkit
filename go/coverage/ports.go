package coverage

import "fmt"

func init() {
	RegisterPort("propagate.coverage", 15001)
}

var ports = make(map[string]int)

func RegisterPort(name string, number int) {
	if _, ok := ports[name]; ok {
		panic(fmt.Sprintf("Port name '%s' already registered", name))
	}
	for k, v := range ports {
		if v == number {
			panic(fmt.Sprintf("Port number %d already registered with key '%s'", v, k))
		}
	}
	ports[name] = number
}

func ResolvePort(key string) int {
	if v, ok := ports[key]; ok {
		return v
	} else {
		panic(fmt.Sprintf("key %s not found", key))
	}
}
