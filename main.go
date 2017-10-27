package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/ahmdrz/goinsta"
)

func main() {

	//@TODO Add limiter flags
	users := flag.String("users", "", "Comma separated list of usernames to scrape")
	configPath := flag.String("config", "", "Path of config file")
	depth := flag.Int("depth", 0, "Max depth to crawl")
	output := flag.String("output", "./", "path to store data")
	flag.Parse()

	err := createDir(*output)
	if err != nil {
		log.Fatalln("Error with output dir: %s", err.Error())
	}

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
		dir:     *output,
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

func createDir(path string) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		log.Printf("Creating path: %s \n", path)
		err = os.MkdirAll(path, 0700)
		if err != nil {
			return err
		}
		return nil
	}
	return err
}
