package main

import (
	"bytes"
	"fmt"
	"testing"
)

func TestRuri(t *testing.T) {
	ruri := DefaultRuriInfo()
	buf := &bytes.Buffer{}
	err := RenderRuriInfo(ruri, buf)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(buf.String())
}
