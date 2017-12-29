package main

import (
	"database/sql"
	"log"

	"time"

	"github.com/ahmdrz/goinsta/response"
	bolt "github.com/johnnadratowski/golang-neo4j-bolt-driver"
)

type dataStore struct {
	sql *sql.DB
	neo bolt.Conn
}

func newDataStore(neoAuth string, mysqlAuth string) *dataStore {
	driver := bolt.NewDriver()
	neo, err := driver.OpenNeo(neoAuth)
	if err != nil {
		log.Fatal("Unable to connect to neo4j")
	}

	db, err := sql.Open("mysql", mysqlAuth)
	if err != nil {
		log.Println("unable to open database.")
	}

	return &dataStore{sql: db, neo: neo}
}

func (db *dataStore) save(r *instaUser) error {
	if r.child != nil {
		if err := db.saveGraph(r); err != nil && r.child != nil {
			return err
		}
		if err := db.saveFollowerSQL(r.child); err != nil {
			return err
		}
	}

	if err := db.saveUserSQL(r.parent); err != nil {
		return err
	}
	return nil
}

func (db *dataStore) shouldCrawl(userName string) bool {
	rows, err := db.sql.Query("SELECT COUNT(id), last_crawl FROM user WHERE username=? GROUP BY id", userName)
	if err != nil {
		log.Printf("Error determining if should crawl: %s \n", err)
		return false
	}

	defer rows.Close()
	i := 0
	for rows.Next() {
		i++
		var count int
		var crawlDate string
		if err := rows.Scan(&count, &crawlDate); err != nil {
			return false
		}
		if count == 0 {
			return true
		}

		s, err := time.Parse("2006-01-02", crawlDate)
		if err != nil {
			log.Printf("unable to parse time: %s \n", err)
			return false
		}

		return time.Now().After(s.Add(24 * time.Hour))
	}
	if i == 0 {
		return true
	}
	return false
}

func (db *dataStore) Close() {
	db.neo.Close()
	db.sql.Close()
}

func (db *dataStore) saveUserSQL(r response.GetUsernameResponse) error {
	stmt, err := db.sql.Prepare(`
		REPLACE INTO user (
						id,
						external_lynx_url, is_verified, media_count,
						auto_expand_chaining, is_favorite, full_name,
						following_count, external_url,
						follower_count, has_anonymous_profile_picture, usertags_count,
						username, geo_media_count, is_business,
						biography, has_chaining, last_crawl)
		VALUES (
			?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	t := time.Now().Format("2006-01-02")
	u := r.User
	_, err = stmt.Exec(
		u.ID,
		u.ExternalLynxURL, u.IsVerified, u.MediaCount,
		u.AutoExpandChaining, u.IsFavorite, u.FullName,
		u.FollowingCount, u.ExternalURL,
		u.FollowerCount, u.HasAnonymousProfilePicture, u.UserTagsCount,
		u.Username, u.GeoMediaCount, u.IsBusiness,
		u.Biography, u.HasChaining, t)

	if err != nil {
		log.Printf("unable to save user data to mysql: %s \n", err)
		return err
	}

	return nil
}

func (db *dataStore) saveFollowerSQL(r *response.User) error {

	// ignore errors that would occur on dupes
	stmt, err := db.sql.Prepare(`
		REPLACE INTO insta.user (
							id,
							is_verified, is_favorite, full_name,
							has_anonymous_profile_picture, username, last_crawl)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	t := time.Now().Format("2006-01-02")
	stmt.Exec(
		r.ID,
		r.IsVerified, r.IsFavorite, r.FullName,
		r.HasAnonymousProfilePicture, r.Username, t,
		t)
	return nil
}

func (db *dataStore) saveGraph(r *instaUser) error {
	stmt, err := db.neo.PrepareNeo(`
						MERGE (a:user {id: {id_a}})
						MERGE (b:user {id: {id_b}})
						MERGE (b)-[:FOLLOWS]->(a);
					`)
	if err != nil {
		log.Println(err)
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecNeo(map[string]interface{}{"id_a": r.parent.User.ID, "id_b": r.child.ID})
	if err != nil {
		log.Println(err)
		return err
	}
	return err
}
