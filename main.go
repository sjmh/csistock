package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"

	"github.com/PuerkitoBio/goquery"
)

type Product struct {
	Name  string
	Url   string
	Stock int
}

type UserEnv struct {
	Wishlist string
	Token    string
}

var (
	notify = map[string]string{
		"notification[title]":       "Low Stock Alert",
		"notification[sound]":       "clanging",
		"notification[source_name]": "CSI Stock Notifier",
		"notification[url]":         "http://www.coolstuffinc.com"}
	boxcar_url = "https://new.boxcar.io/api/notifications"
)

func main() {
	userenv, err := getUserEnv()
	if err != nil {
		log.Fatal("Could not get user environment: ", err)
	}
	products := getProducts(userenv.Wishlist)
	msg := getMessage(products)
	if msg != "" {
		alert(userenv.Token, msg)
	}
}

func getUserEnv() (*UserEnv, error) {
	userenv := UserEnv{}
	userenv.Token = os.Getenv("BOXCAR_TOKEN")
	if userenv.Token == "" {
		return nil, fmt.Errorf("BOXCAR_TOKEN environment variable was missing or blank")
	}

	userenv.Wishlist = os.Getenv("CSI_WISHLIST")
	if userenv.Wishlist == "" {
		return nil, fmt.Errorf("CSI_WISHLIST environment variable was missing or blank")
	}
	return &userenv, nil
}

func getProducts(wishlist_url string) []*Product {
	var doc *goquery.Document
	var e error
	var products []*Product

	if doc, e = goquery.NewDocument(wishlist_url); e != nil {
		log.Fatal(e)
	}

	num_pages := 0

	jobs := make(chan *Product)
	done := make(chan bool)

	pages := doc.Find(".pages").First()
	pages.Find("a").Each(func(i int, s *goquery.Selection) {
		purl := fmt.Sprintf("%s&page=%d", wishlist_url, i+1)
		go pageParser(jobs, done, purl)
		num_pages++
	})

	pages_done := 0
	for {
		select {
		case product := <-jobs:
			products = append(products, product)
		case <-done:
			pages_done++
			if pages_done == num_pages {
				return products
			}
		}
	}
}

func getMessage(products []*Product) string {
	var msg string
	for p := range products {
		if products[p].Stock < 6 && products[p].Stock > 0 {
			msg = msg + fmt.Sprintf("<p><strong><a href=\"%s\">%s</a></strong> is low on stock! Only <strong>%d</strong> left.</p>", products[p].Url, products[p].Name, products[p].Stock)
		}
	}
	return msg
}

func setPost(token string, msg string) *url.Values {
	data := url.Values{}
	data.Set("notification[long_message]", msg)
	data.Set("user_credentials", token)
	for k, v := range notify {
		data.Add(k, v)
	}
	return &data
}

func alert(token string, msg string) {
	data := setPost(token, msg)
	_, err := http.PostForm(boxcar_url, *data)
	if err != nil {
		log.Fatal(err)
	}
}

func pageParser(job chan *Product, done chan bool, purl string) {
	var doc *goquery.Document
	var e error

	if doc, e = goquery.NewDocument(purl); e != nil {
		log.Fatal(e)
	}

	doc.Find("[pid]").Each(func(i int, s *goquery.Selection) {
		p := Product{}
		link := s.Find("h3").Find("a").First()
		p.Name = link.Text()
		p.Url, _ = link.Attr("href")
		p.Stock, e = getStock(s.Find(".stockStatus").Text())
		if e != nil {
			fmt.Printf("Could not find stock for '%s' - %s\n", p.Name, e)
		} else {
			if shouldAlert(s) {
				job <- &p
			}
		}
	})
	done <- true
}

func shouldAlert(s *goquery.Selection) bool {
	qty, _ := regexp.Compile("Wishing for ([0-9]+) of these")
	wish_qty := s.Find(".qtyDesired").Text()

	if m := qty.FindStringSubmatch(wish_qty); m != nil {
		if m[1] == "99" {
			return false
		}
		return true
	}
	return true
}

func getStock(stock string) (int, error) {
	outstock, _ := regexp.Compile("(Out of stock|Pre-order)")
	lowstock, _ := regexp.Compile("Only ([0-9]) left in stock")
	instock, _ := regexp.Compile(`([0-9]+)\+? in stock`)
	matches := []string{}

	switch {
	case outstock.Match([]byte(stock)):
		return 0, nil
	case lowstock.Match([]byte(stock)):
		matches = lowstock.FindStringSubmatch(stock)
	case instock.Match([]byte(stock)):
		matches = instock.FindStringSubmatch(stock)
	}

	if len(matches) > 0 {
		return strconv.Atoi(matches[1])
	}

	return -1, fmt.Errorf("No match found for '%s'", stock)
}
