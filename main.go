package main

import (
	"context"
	"flag"
	"fmt"
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

	if err := createDir(*output); err != nil {
		log.Fatalln("Error with output dir: %s", err.Error())
	}

	limit := rate.Every(time.Second * 2)
	limiter := rate.NewLimiter(limit, 10)

	config, err := getConfig(*configPath)
	if err != nil {
		log.Fatalln(err)
	}

	userNames := strings.Split(*users, ",")
	// User names to crawl
	userChan := make(chan string)

	logFile, err := os.Create(fmt.Sprintf("%s/instacrawl.log", *output))
	if err != nil {
		log.Fatal("unable to create log file")
	}
	logger := log.New(logFile, "", log.Ldate|log.Ltime|log.Llongfile)

	instagram := goinsta.New(config.Username, config.Password)

	crawler := instagramCrawler{
		depth:   *depth,
		service: instagram,
		limiter: limiter,
		mutex:   &sync.Mutex{},
		dir:     *output,
		log:     logger,
	}

	if len(*users) == 0 {
		crawler.log.Fatalln("No usernames provided to scrape.")
	}

	go func() {
		for _, userName := range userNames {
			userChan <- userName
			crawler.log.Printf("Added %s to userChan \n", userName)
		}
	}()

	if err := crawler.service.Login(); err != nil {
		crawler.log.Fatalln("Unable to log in")
	}

	if crawler.service.IsLoggedIn == false {
		crawler.log.Fatalln("Not logged in")
	}

	ctx := context.Background()

	seen := make(map[string]bool)

	guard := false
	for guard != true {
		select {

		case userName := <-userChan:
			if userName == "" {
				continue
			}
			if seen[userName] == false {
				err := limiter.Wait(ctx)
				if err != nil {
					crawler.log.Fatalln(err)
				}
				go crawler.crawl(ctx, userName, userChan)
				seen[userName] = true
			} else {
				crawler.log.Printf("Already crawled: %s \n", userName)
			}
		case <-ctx.Done():
			crawler.log.Println(ctx.Err())
			guard = true
		}
	}

	if err := crawler.service.Logout(); err != nil {
		crawler.log.Println("unable to logout")
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
