package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

//@TODO fix logic for adding to dbChan and rate limiting for instagram

func main() {

	users := flag.String("users", "", "Comma separated list of usernames to scrape")
	configPath := flag.String("config", "", "Path of config file")
	depth := flag.Int("depth", 2, "Max depth to crawl")
	output := flag.String("output", "./", "Path to store data")
	flag.Parse()

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

	// User names to crawl
	userChan := make(chan string)
	dbChan := make(chan *instaUser)

	data := newDataStore(config.NeoAuth, config.MySQLAuth)
	defer data.Close()

	for _, userName := range userNames {
		userChan <- userName
	}

	for {
		select {

		case userName := <-userChan:
			if data.shouldCrawl(userName) {
				go crawler.crawl(context.Background(), userName, userChan, dbChan)
			}
		case r := <-dbChan:
			if err := data.save(r); err != nil {
				crawler.log.Println(err)
			}
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
