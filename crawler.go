package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/ahmdrz/goinsta"
	"github.com/ahmdrz/goinsta/response"
)

type InstagramCrawler struct {
	service  *goinsta.Instagram
	limiter  *rate.Limiter
	mutex    *sync.Mutex
	depth    int
	curDepth int
	dir      string
	log      *log.Logger
}

func (crawler *InstagramCrawler) getFollowers(userChan chan<- string, userID int64, maxID string) {
	crawler.limiter.Wait(context.Background())
	crawler.mutex.Lock()
	followerResp, err := crawler.service.UserFollowers(userID, maxID)
	crawler.mutex.Unlock()
	if err != nil {
		crawler.log.Fatalln(err)
	}
	if len(followerResp.Users) > 0 {
		go func() {
			for _, followers := range followerResp.Users {
				if followers.Username != "" {
					userChan <- followers.Username
					crawler.log.Printf("Added %s to userChan \n", followers.Username)
				}
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

func (crawler *InstagramCrawler) crawl(ctx context.Context, userName string, userChan chan<- string) {
	//@TODO fix ctx
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(1*time.Second))
	defer cancel()
	crawler.mutex.Lock()
	resp, err := crawler.service.GetUserByUsername(userName)
	crawler.mutex.Unlock()
	if err != nil {
		crawler.log.Printf("unable to get user info for %s \n", userName)
		crawler.log.Fatalln(err)
	}
	if resp.Status != "ok" {
		crawler.log.Fatalln(resp.Status)
	}

	go crawler.saveUserToFile(resp)
	go crawler.saveUserPhoto(resp)

	crawler.mutex.Lock()
	if crawler.curDepth <= crawler.depth {
		go crawler.getFollowers(userChan, resp.User.ID, "")
		crawler.curDepth++
	}
	crawler.mutex.Unlock()

	return
}

func (crawler *InstagramCrawler) saveUser(w io.Writer, resp response.GetUsernameResponse) {
	data, err := json.Marshal(resp.User)
	if err != nil {
		crawler.log.Fatalln(err)
	}
	_, err = w.Write(data)
	if err != nil {
		crawler.log.Fatalln(err)
	}
}

func (crawler *InstagramCrawler) saveUserToFile(resp response.GetUsernameResponse) {
	instaUser := resp.User
	userPath := path.Join(crawler.dir, instaUser.Username)
	err := os.MkdirAll(userPath, 0700)
	if err != nil {
		crawler.log.Fatalln(err)
	}

	fileName := fmt.Sprintf("%v-%s.json", time.Now().Unix(), instaUser.Username)
	filePath := path.Join(userPath, fileName)

	f, err := os.Create(filePath)
	if err != nil {
		crawler.log.Fatalln(err)
	}
	crawler.saveUser(f, resp)
}

func (crawler *InstagramCrawler) saveUserPhoto(r response.GetUsernameResponse) {
	resp, err := http.Get(r.User.HdProfilePicURLInfo.URL)
	if err != nil {
		crawler.log.Fatalln(err)
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		crawler.log.Fatalln(err)
	}

	p := path.Join(crawler.dir, r.User.Username)
	ioutil.WriteFile(fmt.Sprintf("%s/%v-%s.jpg", p, time.Now().Unix(), r.User.Username), data, 0700)
}
