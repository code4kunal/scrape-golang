package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gocolly/colly"
	"github.com/gocolly/colly/proxy"
)

var searchURL = "http://shop.holabirdsports.com/search?w="
var debug = flag.Bool("debug", false, "Enable debug output")

type data struct {
	Shoes map[string]struct {
		Options []struct {
			Size string `json:"label"`
		} `json:"options"`
	} `json:"attributes"`
}

func scrape(writer *csv.Writer, keyword, proxies string) {
	c := colly.NewCollector()
	if proxies != "" {
		if p, err := proxy.RoundRobinProxySwitcher(strings.Split(proxies, ",")...); err == nil {
			c.SetProxyFunc(p)
		}
	}
	d := c.Clone()

	c.OnError(func(r *colly.Response, e error) {
		fmt.Println("something went wrong:", r.StatusCode, e, r.Request.URL)
	})

	c.OnHTML(".product-image", func(e *colly.HTMLElement) {
		detailPageURL := e.Attr("href")
		if *debug {
			log.Println("visiting", detailPageURL)
		}
		d.Visit(detailPageURL)
	})

	c.OnHTML(".next-page > a", func(e *colly.HTMLElement) {
		c.Visit(e.Attr("href"))
	})

	d.OnHTML("html", func(e *colly.HTMLElement) {
		priceEndText := e.ChildText(".price_check")
		priceText := strings.Replace(e.ChildText(".add-to-cart-price"), priceEndText, "", 1)
		priceText = strings.Replace(priceText, e.ChildText(".our_price_text"), "", 1)
		prices := strings.Fields(priceText)
		price := prices[len(prices)-1]

		attributes := e.ChildText("#product-options-wrapper script:nth-of-type(2)")
		attributes = attributes[strings.Index(attributes, "{") : strings.LastIndex(attributes, "}")+1]
		if !strings.HasPrefix(price, "$") {
			price = e.ChildText(".msrp_price")
			if !strings.HasPrefix(price, "$") {
				if *debug {
					fmt.Println("No price found, skipping", e.Request.URL)
				}
				return
			}
		}
		title := e.ChildAttr(`meta[name="twitter:title"]`, "content")
		gender := ""
		name := ""
		color := ""
		var titleParts []string
		if strings.Contains(title, "Women's") {
			titleParts = strings.Split(title, "Women's")
			gender = "Woman"
		} else {
			titleParts = strings.Split(title, "Men's")
			gender = "Man"
		}
		name = titleParts[0]
		color = titleParts[1]

		brandTxt := e.ChildText("#google_smart_pixel_beta script")
		brandIdx := -1
		brandParts := strings.Fields(brandTxt)
		for i, v := range brandParts {
			if v == "Brand" {
				brandIdx = i + 2
			}
		}
		brand := brandParts[brandIdx]
		brand = brand[1 : len(brand)-2]

		d := &data{}
		err := json.Unmarshal([]byte(attributes), d)
		if err != nil {
			if *debug {
				fmt.Println("cannot decode json:", err)
			}
			return
		}

		width := e.ChildText("#product-options-wrapper dd:nth-of-type(2)")

		for _, attrs := range d.Shoes {
			for _, a := range attrs.Options {
				writer.Write([]string{
					keyword,
					brand,
					name,
					price,
					e.Request.URL.String(),
					e.ChildAttr("#product_image_anchor img:first-of-type", "src"),
					a.Size,
					width,
					color,
					gender,
					"holabird sports", // retailer
				})
			}
		}
		os.Stderr.Write([]byte("."))

	})
	c.Visit(searchURL + url.QueryEscape(keyword))
}

func main() {
	fName := flag.String("filename", "holabird_sports_shoes"+time.Now().Format("_2006_01_02_15_04_05")+".csv", "Output file name")
	proxies := flag.String("proxies", "", "Comma separated list of proxies (optional)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s [keywords]:\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, `  [keywords] list of search terms. E.g.: "Nike Pegasus"`)
		fmt.Fprintln(os.Stderr, "")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	file, err := os.Create(*fName)
	if err != nil {
		log.Fatalf("Cannot create file %q: %s\n", fName, err)
		return
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	// Write CSV header
	writer.Write([]string{
		"Keyword",
		"Brand",
		"Shoe",
		"Price",
		"URL",
		"Image URL",
		"Size",
		"Width",
		"Color",
		"Gender",
		"Retailer",
	})

	for _, keyword := range flag.Args() {
		log.Println("scraping", keyword)
		scrape(writer, strings.TrimSpace(keyword), *proxies)
		log.Println("done")
	}

}
