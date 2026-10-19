package collector

import (
	"os"
	"strconv"
	"strings"
)

// fields reads path and splits its contents on whitespace.
func fields(path string) ([]string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	return strings.Fields(string(b)), nil
}

// ints parses each string in s as a base-10 uint64.
func ints(s []string) ([]uint64, error) {
	o := make([]uint64, 0, len(s))
	for _, x := range s {
		n, e := strconv.ParseUint(x, 10, 64)
		if e != nil {
			return nil, e
		}
		o = append(o, n)
	}
	return o, nil
}
