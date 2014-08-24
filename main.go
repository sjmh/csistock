package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type Product struct {
	Name         string
	Url          string
	CurrentStock int
	OldStock     int
	Alerted      int64
}

type Products struct {
	Items map[string]*Product
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

func deleteOld(p *Products, np *Products) *Products {
	for k, _ := range p.Items {
		if _, ok := np.Items[k]; !ok {
			delete(p.Items, k)
		}
	}
	return p
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

func getProducts(wishlist_url string) *Products {
	var doc *goquery.Document
	var e error
	products := &Products{Items: make(map[string]*Product)}

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
			products.Items[product.Name] = product
		case <-done:
			pages_done++
			if pages_done == num_pages {
				return products
			}
		}
	}
}

func timePassed(alerted int64) bool {
	if time_since := time.Now().Unix() - alerted; time_since < 86400 {
		return false
	}
	return true
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

func sendAlert(token string, msg string) {
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
		p := new(Product)
		link := s.Find("h3").Find("a").First()
		p.Name = link.Text()
		p.Url, _ = link.Attr("href")
		p.CurrentStock, e = getStock(s.Find(".stockStatus").Text())
		if e != nil {
			fmt.Printf("Could not find stock for '%s' - %s\n", p.Name, e)
		} else {
			job <- p
		}
	})
	done <- true
}

func shouldAlert(p *Product) string {
	if p.OldStock == -1 && p.CurrentStock < 0 {
		return fmt.Sprintf("<p><strong><a href=\"%s\">%s</a></strong> is now available for pre-order!</p>", p.Url, p.Name)
	} else if p.OldStock == 0 && p.CurrentStock < 0 {
		return fmt.Sprintf("<p><strong><a href=\"%s\">%s</a></strong> is now available for ordering!</p>", p.Url, p.Name)
	} else if p.CurrentStock > 0 && p.CurrentStock < 6 {
		if p.CurrentStock < p.OldStock || timePassed(p.Alerted) {
			return fmt.Sprintf("<p><strong><a href=\"%s\">%s</a></strong> has only <strong>%d</strong> copies left.</p>", p.Url, p.Name, p.CurrentStock)
		}
	} else if p.CurrentStock == 0 && p.OldStock != 0 {
		return fmt.Sprintf("<p><strong><a href=\"%s\">%s</a></strong> is now out of stock.</p>", p.Url, p.Name)
	}
	return ""
}

func getStock(stock string) (int, error) {
	outstock, _ := regexp.Compile("Out of stock")
	preorder, _ := regexp.Compile("Pre-order")
	lowstock, _ := regexp.Compile("Only ([0-9]) left in stock")
	instock, _ := regexp.Compile(`([0-9]+)\+? in stock`)
	matches := []string{}

	switch {
	case outstock.Match([]byte(stock)):
		return 0, nil
	case preorder.Match([]byte(stock)):
		return -1, nil
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

func main() {
	userenv, err := getUserEnv()
	if err != nil {
		log.Fatal("Could not get user environment: ", err)
	}
	products := &Products{Items: make(map[string]*Product)}
	var msg string
	for {
		nproducts := getProducts(userenv.Wishlist)
		for npName, np := range nproducts.Items {
			if _, ok := products.Items[npName]; !ok {
				products.Items[npName] = np
			}
			p := products.Items[npName]
			p.OldStock = p.CurrentStock
			p.CurrentStock = np.CurrentStock
			if alert := shouldAlert(p); alert != "" {
				p.Alerted = time.Now().Unix()
				msg += alert
			}
		}
		products = deleteOld(products, nproducts)
		if msg != "" {
			sendAlert(userenv.Token, msg)
			msg = ""
		}
		time.Sleep(time.Minute * 5)
	}
}
