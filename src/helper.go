package main

import (
	"log"
	"net/http"
	"strings"
)

func checkErr(err error) {
	if err != nil {
		log.Panicln(err)
	}
}

func checkStatusCode(res *http.Response) {
	if res.StatusCode != 200 {
		log.Panicln("Request failed with Status:", res.StatusCode)
	}
}

func cleanString(str string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(str)), " ")
}
