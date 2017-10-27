package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/user"
	"path"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/ahmdrz/goinsta"
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

	//@TODO Add limiter flags
	users := flag.String("users", "", "Comma separated list of usernames to scrape")
	configPath := flag.String("config", "", "Path of config file")
	depth := flag.Int("depth", 0, "Max depth to crawl")
	flag.Parse()

	limit := rate.Every(time.Second * 2)
	limiter := rate.NewLimiter(limit, 10)

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
		depth:   *depth,
		service: instagram,
		limiter: limiter,
		mutex:   &sync.Mutex{},
		dir:     dir,
	}

	if err := crawler.service.Login(); err != nil {
		log.Fatalln("Unable to log in")
	}
	defer crawler.service.Logout()

	if crawler.service.IsLoggedIn == false {
		log.Fatalln("Not logged in")
	}

	ctx := context.Background()

	seen := make(map[string]bool)

	for {
		select {

		case userName := <-userChan:
			if userName == "" {
				continue
			}
			if seen[userName] == false {
				err := limiter.Wait(ctx)
				if err != nil {
					log.Fatalln(err)
				}
				go crawler.crawl(ctx, userName, userChan)
				seen[userName] = true
			} else {
				log.Printf("Already crawled: %s \n", userName)
			}
		case <-ctx.Done():
			log.Println(ctx.Err())
		}
	}
}
