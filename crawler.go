package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/ahmdrz/goinsta"
	"golang.org/x/time/rate"
)

type InstagramCrawler struct {
	service *goinsta.Instagram
	limiter *rate.Limiter
	mutex   *sync.Mutex
}

func (crawler *InstagramCrawler) getFollowers(userChan chan<- string, userID int64, maxID string) {
	crawler.limiter.Wait(context.Background())
	crawler.mutex.Lock()
	followerResp, err := crawler.service.UserFollowers(userID, maxID)
	crawler.mutex.Unlock()
	if err != nil {
		log.Fatalln(err)
	}
	if len(followerResp.Users) > 0 {
		go func() {
			for _, followers := range followerResp.Users {
				userChan <- followers.Username
				log.Printf("Added %s to userChan \n", followers.Username)
			}
			// no need to crawl followers of followers yet
			close(userChan)
		}()
	} else {
		return
	}

	if followerResp.NextMaxID != "" {
		crawler.getFollowers(userChan, userID, followerResp.NextMaxID)
	}
}

func (crawler InstagramCrawler) crawl(ctx context.Context, userName string, userChan chan<- string) {
	//@TODO fix ctx
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(1*time.Second))
	defer cancel()
	crawler.mutex.Lock()
	resp, err := crawler.service.GetUserByUsername(userName)
	crawler.mutex.Unlock()
	if err != nil {
		log.Fatalf("unable to get user info for %s \n", userName)
	}
	if resp.Status != "ok" {
		log.Fatalln(resp.Status)
	}

	//@TODO add save to struct
	go saveUserToFile(resp)

	go crawler.getFollowers(userChan, resp.User.ID, "")

	return
}
