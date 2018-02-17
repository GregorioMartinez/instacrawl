package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {

	users := flag.String("users", "", "Comma separated list of usernames to scrape")
	configPath := flag.String("config", "", "Path of config file")
	depth := flag.Int("depth", 2, "Max depth to crawl")
	output := flag.String("output", "./", "Path to store data")
	verbose := flag.Bool("verbose", false, "Verbose output")
	label := flag.String("label", "", "Manually set users as real or fake")
	source := flag.String("source", "", "Source of initial seed")
	duration := flag.String("timeout", "300s", "Timeout in seconds before terminating")

	flag.Parse()

	timeout, err := time.ParseDuration(*duration)
	if err != nil {
		log.Fatal(err)
	}

	config, err := getConfig(*configPath)
	if err != nil {
		log.Fatalln(err)
	}

	crawler := newCrawler(config, 2, *depth, *output)

	if err := crawler.service.Login(); err != nil {
		crawler.log.Fatalln("Unable to log in")
	}

	userNames := strings.Split(*users, ",")
	if len(userNames) == 0 {
		crawler.log.Fatalln("No usernames provided to scrape.")
	}

	userChan := make(chan string)
	dbChan := make(chan *instaUser)

	data := newDataStore(config.NeoAuth, config.MySQLAuth)
	defer data.Close()

	go func() {
		for _, userName := range userNames {
			if *verbose {
				log.Printf("Added %s to userChan", userName)
			}
			userChan <- userName
		}
	}()

	guard := false
	for !guard {
		select {
		case userName := <-userChan:
			if data.shouldCrawl(userName) {
				if *verbose {
					log.Printf("Crawling %s", userName)
				}
				go crawler.crawl(context.Background(), userName, userChan, dbChan)
			}
		case r := <-dbChan:
			if *verbose {
				if r.child != nil {
					log.Printf("Saving %s : %s to database", r.parent.User.Username, r.child.Username)
				} else {
					log.Printf("Saving %s to database", r.parent.User.Username)
				}
			}
			if err := data.save(r, *label, *source); err != nil {
				crawler.log.Println(err)
			}
		case <-time.Tick(timeout):
			log.Printf("%v with no activity. Shutting down \n", *duration)
			guard = true
		}
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
