package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gocolly/colly"
	"github.com/gocolly/colly/proxy"
	"encoding/json"
	"reflect"
	"errors"
	"regexp"
)

const searchURL = `https://www.eastbay.com/Running/Shoes/_-_/N-1dwZne/keyword-%s`
const retailerName = "Eastbay.com"
const productsSelector = "#endeca_search_results > ul > li"
const nextPageSelector = ".endeca_pagination .next"
const titleSelector = `meta[name="title"]`
const fileNamePrefix = "eastbay_com"
const host = "https://www.eastbay.com"
const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/67.0.3396.79 Safari/537.36"

var debug = flag.Bool("debug", false, "Enable debug output")

type Shoe struct {
	Color string
	Price string
	Width string
	ImageUrl string
	Sizes []Size
}

type Size struct {
	Value string
	Price string
}

func scrape(writer *csv.Writer, keyword, proxies string) {
	productIds := map[string]bool{}
	productListScraper := colly.NewCollector()
	productListScraper.OnRequest(func(request *colly.Request) {
		request.Headers.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8")
		request.Headers.Add("Accept-Language", "en-US,en;q=0.9,ru;q=0.8")
	})
	productListScraper.UserAgent = userAgent
	if proxies != "" {
		if p, err := proxy.RoundRobinProxySwitcher(strings.Split(proxies, ",")...); err == nil {
			productListScraper.SetProxyFunc(p)
		}
	}

	productListScraper.OnError(func(r *colly.Response, e error) {
		fmt.Println("something went wrong:", r.StatusCode, e, r.Request.URL)
	})

	detailPageScraper := productListScraper.Clone()

	productListScraper.OnHTML(productsSelector, func(e *colly.HTMLElement) {

		productId := e.Attr("data-model")
		if *debug {
			log.Println("productId", productId)
		}
		_, ok := productIds[productId]
		if ok {
			return // skip product if it's already scraped
		}

		productIds[productId] = true
		detailPageURL := e.ChildAttr("a:nth-of-type(1)", "href")
		if *debug {
			log.Println("visiting", detailPageURL)
		}
		detailPageScraper.Visit(detailPageURL)
	})

	productListScraper.OnHTML("html", func(e *colly.HTMLElement) {
		if *debug {
			_, err := e.DOM.Html()
			if err == nil {
				log.Println("product list is opened", e.Request.URL)
			} else {
				log.Println("product list is opened, but there is no html", e.Request.URL, err)
			}
		}
	})

	productListScraper.OnHTML(nextPageSelector, func(e *colly.HTMLElement) {
		productListScraper.Visit(host + e.Attr("href"))
	})

	detailPageScraper.OnHTML("html", func(e *colly.HTMLElement) {
		if *debug {
			log.Println("detailPage is opened", e.Request.URL)
		}

		err := onDetailPageOpened(e, writer, keyword)
		if err != nil {
			return
		}
	})
	productListScraper.Visit(fmt.Sprintf(searchURL, url.QueryEscape(keyword)))
}

func onDetailPageOpened(e *colly.HTMLElement, writer *csv.Writer, keyword string) error {

	title := strings.ToLower(e.ChildAttr(titleSelector, "content"))
	if strings.Contains(title, "girls'") || strings.Contains(title, "boys'") || strings.Contains(title, "kids'") {
		return errors.New("kids' shoe")
	}
	brand := extractBrand(e)
	shoes := extractShoes(e)
	gender, name := extractGenderAndName(title)

	for _, shoe := range shoes {
		for _, size := range shoe.Sizes {
			writer.Write([]string{
				keyword,
				brand,
				name,
				size.Price,
				e.Request.URL.String(),
				shoe.ImageUrl,
				size.Value,
				shoe.Width,
				shoe.Color,
				gender,
				retailerName, // retailer
			})
		}
	}
	os.Stderr.Write([]byte("."))
	return nil
}

func extractBrand(e *colly.HTMLElement) string {
	html, err := e.DOM.Html()
	if err != nil {
		return ""
	}
	var brandRegexp = regexp.MustCompile(`tagMgt.brand = "(.*)";`)
	brandName := brandRegexp.FindStringSubmatch(html)
	if len(brandName) > 1 {
		return brandName[1]
	} else {
		return ""
	}
}

func extractShoes(e *colly.HTMLElement) []Shoe {

	shoesJson := e.ChildText(".content_container script:nth-of-type(6)")
	startIndex := strings.Index(shoesJson, "var styles = {")
	shoesJson = shoesJson[startIndex+13:]
	endIndex := strings.Index(shoesJson, "};")
	shoesJson = shoesJson[:endIndex+1]

	siteData := map[string][]interface{}{}
	err := json.Unmarshal([]byte(shoesJson), &siteData)
	if err != nil {
		if *debug {
			fmt.Println("cannot decode json:", err)
		}
	}
	var shoes []Shoe
	for model, attributes := range siteData {
		shoe := Shoe{}
		shoe.Color = attributes[15].(string)
		shoe.Width = attributes[16].(string)
		shoe.Price = attributes[6].(string)
		shoe.ImageUrl = fmt.Sprintf("https://images.eastbay.com/is/image/EBFL2/%s", model)
		sizes := convertInterfaceToArray(attributes[7])
		for _, sizeInterface := range sizes {
			sizeAttributes := convertInterfaceToArray(sizeInterface)
			size := Size{
				Value: strings.Trim(sizeAttributes[0].(string), ` ""`),
				Price: sizeAttributes[2].(string),
			}
			shoe.Sizes = append(shoe.Sizes, size)
		}
		shoes = append(shoes, shoe)
	}

	return shoes
}

func convertInterfaceToArray(t interface{}) []interface{} {
	var result []interface{}
	kind := reflect.TypeOf(t).Kind()
	switch kind {
	case reflect.Slice:

		s := reflect.ValueOf(t)

		for i := 0; i < s.Len(); i++ {
			result = append(result, s.Index(i).Interface())
		}
		break
	}
	return result
}


func extractGenderAndName(title string) (string, string) {
	gender := ""
	name := ""
	if strings.Contains(title, "women's") {
		gender = "Woman"
		name = strings.TrimSuffix(title, " - women's")
	} else {
		gender = "Man"
		name = strings.TrimSuffix(title, " - men's")
	}
	return gender, strings.TrimSpace(name)
}

func main() {
	fName := flag.String("filename", fileNamePrefix+time.Now().Format("_2006_01_02_15_04_05")+".csv", "Output file name")
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
