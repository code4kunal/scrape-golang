package main

import (
	"fmt"
	"os"
	"log"
	"flag"
	"encoding/csv"
	"strings"
	"net/url"
	"time"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/proxy"
	"encoding/json"
)

var searchURL = "https://www.jackrabbit.com/catalogsearch/result/?q="
var debug = flag.Bool("debug", false, "Enable debug output")
var gender string
var color string
var price string
var size string
var width string

type data struct {
	Shoes map[string]struct {
		image string `json:"image"`
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
		fmt.Println(detailPageURL)
		if *debug {
			log.Println("visiting", detailPageURL)
		}
		d.Visit(detailPageURL)
	})

	c.OnHTML(".next i-next", func(e *colly.HTMLElement) {
		c.Visit(e.Attr("href"))
	})

	d.OnHTML("html", func(e *colly.HTMLElement) {

		attributes := e.ChildText("#product-options-wrapper script:nth-of-type(2)")
		attributes = attributes[strings.Index(attributes, "{") : strings.LastIndex(attributes, "}")+1]

		//price
		oldPrice := e.ChildText(".old-price")
		specialPrice := e.ChildText(".special-price")
		regularPrice := e.ChildText(".regular-price")
		if len(regularPrice) == 0 {
			if (len(specialPrice) == 0) {
				price = oldPrice
			} else {
				price = specialPrice
			}
			if !strings.HasPrefix(price, "$") {
				if *debug {
					fmt.Println("No price found, skipping", e.Request.URL)
				}
				return
			}
		} else {
			price = regularPrice
		}
		prices := strings.Split(price, "\n")
		finalPrice := prices[0]

		//Title, brand, gender
		productTitle := e.ChildText("div.product-name > h1")
		tilesAsTokens := strings.Split(productTitle, " ")

		brand := tilesAsTokens[1]
		if strings.Contains(brand, "-") {
			if *debug {
				fmt.Println("Not a valid URL", e.Request.URL)
			}
			return
		}

		genderText := tilesAsTokens[0]
		if (strings.Contains(genderText, "Men")) {
			gender = "Man"
		} else if (strings.Contains(genderText, "Women")) {
			gender = "Woman"
		} else {
			gender = "Unisex"
		}

		d := &data{}
		err := json.Unmarshal([]byte(attributes), d)
		if err != nil {
			if *debug {
				fmt.Println("cannot decode json:", err)
			}
			return
		}

		writer.Write([]string{
			keyword,
			brand,
			productTitle,
			finalPrice,
			e.Request.URL.String(),
			e.ChildAttr(".zoomWrapper img:first-of-type", "src"),
			size,
			width,
			color,
			gender,
			"jack-rabbit", // retailer
		})

	})
	fmt.Println(searchURL + url.QueryEscape(keyword+" SHOES"))
	c.Visit(searchURL + url.QueryEscape(keyword+" SHOES"))
}

func main() {
	fName := flag.String("filename", "jack_rabbit_shoes"+time.Now().Format("_2006_01_02_15_04_05")+".csv", "Output file name")
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

