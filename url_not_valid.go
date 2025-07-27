package main

import "net/url"

func urlNotlValid(URL string) bool {
	_, err := url.Parse(URL)
	return err != nil
}
