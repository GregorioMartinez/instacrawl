package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/ahmdrz/goinsta"
	"github.com/ahmdrz/goinsta/response"
)

var dir string

func init() {
	u, err := user.Current()
	if err != nil {
		log.Fatalln(err)
	}

	dir = path.Join(u.HomeDir, ".instacrawl")

	_, err = os.Stat(dir)
	if os.IsNotExist(err) {
		log.Printf("Creating dir: %s \n", dir)
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			log.Fatalln(err)
		}
	}
}

func main() {

	//@TODO Add depth flag
	//@TODO Add limiter flags
	users := flag.String("users", "", "Comma separated list of usernames to scrape")
	configPath := flag.String("config", "", "Path of config file")
	flag.Parse()

	limit := rate.Every(time.Second * 100)
	limiter := rate.NewLimiter(limit, 100)

	config, err := getConfig(*configPath)
	if err != nil {
		log.Fatalln(err)
	}

	if len(*users) == 0 {
		log.Fatalln("No usernames provided to scrape.")
	}

	userNames := strings.Split(*users, ",")

	// User names to crawl
	userChan := make(chan string)

	go func() {
		for _, userName := range userNames {
			userChan <- userName
			log.Printf("Added %s to userChan \n", userName)
		}
	}()

	instagram := goinsta.New(config.Username, config.Password)

	crawler := InstagramCrawler{
		service: instagram,
		limiter: limiter,
	}

	if err := crawler.service.Login(); err != nil {
		log.Fatalln("Unable to log in")
	}
	defer crawler.service.Logout()

	if crawler.service.IsLoggedIn == false {
		log.Fatalln("Not logged in")
	}

	ctx := context.Background()

	for {
		select {

		case userName := <-userChan:
			err := limiter.Wait(ctx)
			if err != nil {
				log.Fatalln(err)
			}
			go crawl(ctx, crawler, userName, userChan)
		case <-ctx.Done():
			log.Println(ctx.Err())
		}
	}
}

func saveUserToFile(resp response.GetUsernameResponse) {
	instaUser := resp.User
	userPath := path.Join(dir, instaUser.Username)
	err := os.MkdirAll(userPath, 0700)
	if err != nil {
		log.Fatalln(err)
	}

	fileName := fmt.Sprintf("%v-%s.json", time.Now().Unix(), instaUser.Username)
	filePath := path.Join(userPath, fileName)

	data, err := json.Marshal(instaUser)
	if err != nil {
		log.Fatalln(err)
	}
	err = ioutil.WriteFile(filePath, data, 0700)
	if err != nil {
		log.Fatalln(err)
	}
}

func getFollowers(crawler InstagramCrawler, userChan chan<- string, userID int64, maxID string) {
	crawler.limiter.Wait(context.Background())
	followerResp, err := crawler.service.UserFollowers(userID, maxID)
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
		getFollowers(crawler, userChan, userID, followerResp.NextMaxID)
	}
}

func crawl(ctx context.Context, crawler InstagramCrawler, userName string, userChan chan<- string) {
	//@TODO fix ctx
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(1*time.Second))
	defer cancel()
	resp, err := crawler.service.GetUserByUsername(userName)
	if err != nil {
		log.Fatalf("unable to get user info for %s \n", userName)
	}
	if resp.Status != "ok" {
		log.Fatalln(resp.Status)
	}

	go saveUserToFile(resp)

	go getFollowers(crawler, userChan, resp.User.ID, "")

	return
}
