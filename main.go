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
	_ "github.com/go-sql-driver/mysql"
)

//@TODO fix logic for adding to dbChan and rate limiting for instagram

func main() {

	users := flag.String("users", "", "Comma separated list of usernames to scrape")
	configPath := flag.String("config", "", "Path of config file")
	depth := flag.Int("depth", 2, "Max depth to crawl")
	output := flag.String("output", "./", "Path to store data")
	flag.Parse()

	if err := createDir(*output); err != nil {
		log.Fatalln("Error with output dir: %s", err.Error())
	}

	config, err := getConfig(*configPath)
	if err != nil {
		log.Fatalln(err)
	}

	limiter := rate.NewLimiter(rate.Every(time.Second*2), 10)

	logFile, err := os.Create(fmt.Sprintf("%s/instacrawl.log", *output))
	if err != nil {
		log.Fatal("unable to create log file", err)
	}
	logger := log.New(logFile, "", log.Ldate|log.Ltime|log.Llongfile)

	crawler := instagramCrawler{
		depth:   *depth,
		service: goinsta.New(config.Username, config.Password),
		limiter: limiter,
		mutex:   &sync.Mutex{},
		dir:     *output,
		log:     logger,
	}

	if err := crawler.service.Login(); err != nil {
		crawler.log.Fatalln("Unable to log in")
	}

	if !crawler.service.IsLoggedIn {
		crawler.log.Fatalln("Not logged in")
	}

	userNames := strings.Split(*users, ",")
	if len(userNames) == 0 {
		crawler.log.Fatalln("No usernames provided to scrape.")
	}

	// User names to crawl
	userChan := make(chan string)
	dbChan := make(chan *instaUser)

	data := newDataStore(config.NeoAuth, config.MySQLAuth)

	go func() {
		for _, userName := range userNames {
			userChan <- userName
		}
	}()

	for {
		select {

		case userName := <-userChan:
			if userName == "" {
				continue
			}
			if data.shouldCrawl(userName) {
				if err := crawler.limiter.Wait(context.Background()); err != nil {
					crawler.log.Fatalln(err)
				}
				go crawler.crawl(context.Background(), userName, userChan, dbChan)
			}
		case r := <-dbChan:
			if err := data.save(r); err != nil {
				crawler.log.Println(err)
			}
		}
	}

	if err := crawler.service.Logout(); err != nil {
		crawler.log.Println("unable to logout")
	}

	if err := data.neo.Close(); err != nil {
		crawler.log.Fatal("Unable to close neo4j connection")
	}

	if err := data.sql.Close(); err != nil {
		crawler.log.Fatal("Unable to close neo4j connection")
	}
}

func createDir(path string) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		log.Printf("Creating path: %s \n", path)
		err = os.MkdirAll(path, 0700)
	}
	return err
}
