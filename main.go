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
	Pid          string
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
		//"notification[title]":       "Low Stock Alert",
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

func getProducts(wishlist_url string) (*Products, error) {
	var doc *goquery.Document
	var e error
	products := &Products{Items: make(map[string]*Product)}

	log.Println("Making query to CSI website")
	if doc, e = goquery.NewDocument(wishlist_url); e != nil {
		return nil, e
	}

	num_pages := 0

	jobs := make(chan *Product)
	done := make(chan bool)
	timeout := make(chan bool, 1)

	log.Println("Parsing returned wishlist")
	pages := doc.Find(".pages").First()
	if pages.Length() == 0 {
		return nil, fmt.Errorf("No pages were returned")
	}

	go func() {
		time.Sleep(60 * time.Second)
		timeout <- true
	}()

	pages.Find("a").Each(func(i int, s *goquery.Selection) {
		log.Printf("Executing go routine for page %d", i+1)
		purl := fmt.Sprintf("%s&page=%d", wishlist_url, i+1)
		go pageParser(jobs, done, purl)
		num_pages++
	})
	if num_pages == 0 {
		return nil, fmt.Errorf("No pages were parsed from output")
	}

	log.Println("Waiting for pages to complete parsing")
	pages_done := 0
	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("Timed out while waiting for wishlist to be parsed")
		case product := <-jobs:
			products.Items[product.Pid] = product
		case <-done:
			pages_done++
			if pages_done == num_pages {
				return products, nil
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
	data.Set("notification[title]", msg)
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
		p.Pid, _ = s.Attr("pid")
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
	if p.CurrentStock == -1 && p.OldStock != -1 {
		return fmt.Sprintf("%s is available for pre-order", p.Name)
	} else if p.CurrentStock > 0 && p.OldStock <= 0 {
		return fmt.Sprintf("%s now has %d copies available", p.Name, p.CurrentStock)
	} else if p.CurrentStock == 0 && p.OldStock != 0 {
		return fmt.Sprintf("%s is out of stock", p.Name)
	} else if p.CurrentStock > 0 && p.CurrentStock < 6 {
		if p.CurrentStock < p.OldStock {
			return fmt.Sprintf("%s down to %d copies left", p.Name, p.CurrentStock)
		}
		// Taking out time based alerting
		// else if timePassed(p.Alerted) {
		//	return fmt.Sprintf("<p><strong><a href=\"%s\">%s</a></strong> still has <strong>%d</strong> copies left. (%s)</p>", p.Url, p.Name, p.CurrentStock, time.Unix(p.Alerted, 0))
		//}
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
	f, err := os.Create("csi.log")
	defer f.Close()
	log.SetOutput(f)
	log.SetFlags(3)

	for {
		log.Println("Starting run")
		nproducts, err := getProducts(userenv.Wishlist)
		if err != nil {
			log.Printf("Error while getting products: %v\n", err)
			time.Sleep(time.Minute * 5)
			continue
		}
		log.Println("Parsing products")
		for pid, np := range nproducts.Items {
			// if key doesn't exist in products, add it
			if _, ok := products.Items[pid]; !ok {
				products.Items[pid] = np
			}
			p := products.Items[pid]
			p.OldStock = p.CurrentStock
			p.CurrentStock = np.CurrentStock
			if alert := shouldAlert(p); alert != "" {
				// Don't need time.
				//p.Alerted = time.Now().Unix()
				sendAlert(userenv.Token, alert)
			}
		}
		log.Println("Deleting old products")
		products = deleteOld(products, nproducts)
		log.Println("Printing products")
		for _, p := range products.Items {
			log.Printf("%s, %d, %d, %d\n", p.Name, p.OldStock, p.CurrentStock, p.Alerted)
		}
		if msg != "" {
			msg = ""
		}
		time.Sleep(time.Minute * 5)
	}
}
