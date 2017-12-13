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
	"github.com/johnnadratowski/golang-neo4j-bolt-driver"
)

type instagramCrawler struct {
	service  *goinsta.Instagram
	limiter  *rate.Limiter
	mutex    *sync.Mutex
	depth    int
	curDepth int
	dir      string
	log      *log.Logger
	neo      golangNeo4jBoltDriver.Conn
}

type instaUser struct {
	parent response.GetUsernameResponse
	child  *response.User
}

func newCrawler(config config, limit time.Duration, depth int, output string) *instagramCrawler {

	limiter := rate.NewLimiter(rate.Every(time.Second*limit), 10)

	if err := createDir(output); err != nil {
		log.Fatalln("Error with output dir: %s", err.Error())
	}

	logFile, err := os.Create(fmt.Sprintf("%s/instacrawl.log", output))
	if err != nil {
		log.Fatal("unable to create log file", err)
	}
	logger := log.New(logFile, "", log.Ldate|log.Ltime|log.Llongfile)

	return &instagramCrawler{
		depth:   depth,
		service: goinsta.New(config.Username, config.Password),
		limiter: limiter,
		mutex:   &sync.Mutex{},
		dir:     output,
		log:     logger,
	}
}

func (crawler *instagramCrawler) getFollowers(userChan chan<- string, dbChan chan *instaUser, resp response.GetUsernameResponse, maxID string) {
	if err := crawler.limiter.Wait(context.Background()); err != nil {
		crawler.log.Println("error waiting")
	}
	crawler.mutex.Lock()
	followerResp, err := crawler.service.UserFollowers(resp.User.ID, maxID)
	crawler.mutex.Unlock()
	if err != nil {
		crawler.log.Fatalln(err)
	}
	if len(followerResp.Users) > 0 {
		go func() {
			for _, followers := range followerResp.Users {
				if followers.Username != "" {
					userChan <- followers.Username
					dbChan <- &instaUser{
						parent: resp,
						child:  &followers,
					}
				}
			}
		}()
	}

	if followerResp.NextMaxID != "" {
		if err := crawler.limiter.Wait(context.TODO()); err != nil {
			crawler.getFollowers(userChan, dbChan, resp, followerResp.NextMaxID)
		}
	}
}

func (crawler *instagramCrawler) crawl(ctx context.Context, userName string, userChan chan<- string, dbChan chan *instaUser) {
	//@TODO fix ctx
	//ctx, cancel := context.WithDeadline(ctx, time.Now().Add(1*time.Second))
	//defer cancel()
	if err := crawler.limiter.Wait(context.TODO()); err != nil {
		crawler.log.Fatalln(err)
	}
	crawler.mutex.Lock()
	resp, err := crawler.service.GetUserByUsername(userName)
	dbChan <- &instaUser{
		child:  nil,
		parent: resp,
	}
	crawler.mutex.Unlock()
	if err != nil {
		crawler.log.Printf("unable to get user info for %s \n", userName)
		crawler.log.Fatalln(err)
	}
	if resp.Status != "ok" {
		crawler.log.Fatalln(resp.Status)
	}
	crawler.mutex.Lock()
	if crawler.curDepth <= crawler.depth {
		if err := crawler.limiter.Wait(ctx); err != nil {
			go crawler.getFollowers(userChan, dbChan, resp, "")
			crawler.curDepth++
		}
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
