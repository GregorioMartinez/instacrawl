package main

import (
	"context"
	"database/sql"
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

type instagramCrawler struct {
	service  *goinsta.Instagram
	limiter  *rate.Limiter
	mutex    *sync.Mutex
	depth    int
	curDepth int
	dir      string
	log      *log.Logger
	db       *sql.DB
}

func (crawler *instagramCrawler) getFollowers(userChan chan<- string, userID int64, maxID string) {
	if err := crawler.limiter.Wait(context.Background()); err != nil {
		crawler.log.Println("error waiting")
	}
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

func (crawler *instagramCrawler) crawl(ctx context.Context, userName string, userChan chan<- string) {
	//@TODO fix ctx
	//ctx, cancel := context.WithDeadline(ctx, time.Now().Add(1*time.Second))
	//defer cancel()
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

func (crawler *instagramCrawler) saveUser(w io.Writer, resp response.GetUsernameResponse) {
	data, err := json.Marshal(resp.User)
	if err != nil {
		crawler.log.Fatalln(err)
	}
	_, err = w.Write(data)
	if err != nil {
		crawler.log.Fatalln(err)
	}
}

func (crawler *instagramCrawler) saveUserToFile(resp response.GetUsernameResponse) {
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

func (crawler *instagramCrawler) saveUserPhoto(r response.GetUsernameResponse) {
	resp, err := http.Get(r.User.HdProfilePicURLInfo.URL)
	if err != nil {
		crawler.log.Fatalln(err)
	}

	defer func() {
		if err = resp.Body.Close(); err != nil {
			crawler.log.Printf("error closing body responese: %s \n", err)
		}
	}()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		crawler.log.Fatalln(err)
	}

	p := path.Join(crawler.dir, r.User.Username)
	name := fmt.Sprintf("%s/%v-%s.jpg", p, time.Now().Unix(), r.User.Username)
	err = ioutil.WriteFile(name, data, 0700)
	if err != nil {
		crawler.log.Printf("unable to write %s to file %s", r.User.Username, name)
	}
}
