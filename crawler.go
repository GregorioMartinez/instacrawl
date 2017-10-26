package main

import (
	"github.com/ahmdrz/goinsta"
	"golang.org/x/time/rate"
)

type InstagramCrawler struct {
	service *goinsta.Instagram
	limiter *rate.Limiter
}
