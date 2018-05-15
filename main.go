package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/goware/urlx"
)

var (
	mworkers     = 2            // кол-во "потоков"
	reportPeriod = 10           // частота отчетов (сек)
	dupToStop    = 500          // максимум повторов до останова
	hashFile     = "hash.bin"   // файл с хешами
	quotesFile   = "quotes.txt" // файл с цитатами
	website      = ""
	used         = make(map[string]bool)
)

func init() {
	//Задаем правила разбора:
	flag.IntVar(&mworkers, "w", mworkers, "количество потоков")
	flag.IntVar(&reportPeriod, "r", reportPeriod, "частота отчетов (сек)")
	flag.IntVar(&dupToStop, "d", dupToStop, "кол-во дубликатов для остановки")
	flag.StringVar(&hashFile, "hf", hashFile, "файл хешей")
	flag.StringVar(&quotesFile, "qf", quotesFile, "файл записей")
	flag.StringVar(&website, "ws", website, "адрес сайта для парсинга")
}

func main() {
	//И запускаем разбор аргументов
	flag.Parse()
	if website == "" {
		log.Fatal("Адрес сайта не указан")
	}

	quoteChan := grab()
	for i := 0; i < 10; i++ {
		fmt.Println(<-quoteChan)
	}
}

func grab() <-chan string {
	// функция вернет каналб из которого мы будем читать данные типа string
	c := make(chan string)
	for i := 0; i < mworkers; i++ {
		go func() {
			for {
				res, err := http.Get(website)
				if err != nil {
					log.Fatal(err)
				}
				defer res.Body.Close()
				if res.StatusCode != 200 {
					log.Fatalf("status code error: %d %s", res.StatusCode, res.Status)
				}

				doc, err := goquery.NewDocumentFromReader(res.Body)
				if err != nil {
					log.Fatal(err)
				}

				doc.Find("a").Each(func(i int, s *goquery.Selection) {
					band, ok := s.Attr("href")
					if ok {
						band = strings.TrimSpace(band)
						if !strings.HasPrefix(band, "http") {
							url, _ := urlx.Parse(website)
							mainurl := url.Scheme + "://" + url.Host
							band = mainurl + band
						}
						c <- band
					}
				})

				time.Sleep(100 * time.Millisecond)
			}
		}()
	}
	fmt.Println("Запущено потоков: ", mworkers)
	return c
}
