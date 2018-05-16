package main

import (
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	readHashes()

	qfile, err := os.OpenFile(quotesFile, os.O_APPEND|os.O_CREATE, 0666)
	check(err)
	defer qfile.Close()

	hfile, err := os.OpenFile(hashFile, os.O_APPEND|os.O_CREATE, 0666)
	check(err)
	defer hfile.Close()

	ticker := time.NewTicker(time.Duration(reportPeriod) * time.Second)
	defer ticker.Stop()

	keychan := make(chan os.Signal, 1)
	signal.Notify(keychan, os.Interrupt)

	//...и все что нужно для подсчета хешей
	hasher := md5.New()

	//Счетчики цитат и дубликатов
	quotescount, dupcount := 0, 0

	//Все готово, поехали!
	quotesChan := grab()
	for {
		select {
		case quote := <-quotesChan: //если "пришла" новая цитата:
			quotescount++
			//считаем хеш, и конвертируем его в строку:
			hasher.Reset()
			io.WriteString(hasher, quote)
			hash := hasher.Sum(nil)
			hashstring := hex.EncodeToString(hash)
			//проверяем уникальность хеша цитаты
			if !used[hashstring] {
				//все в порядке - заносим хеш в хранилище, и записываем его и цитату в файлы
				used[hashstring] = true
				hfile.Write(hash)
				qfile.WriteString(quote + "\n\n\n")
				dupcount = 0
			} else {
				//получен повтор - пришло время проверить, не пора ли закругляться?
				if dupcount++; dupcount == dupToStop {
					fmt.Println("Достигнут предел повторов, завершаю работу. Всего записей: ", len(used))
					return
				}
			}
		case <-keychan: //если пришла информация от нотификатора сигналов:
			fmt.Println("CTRL-C: Завершаю работу. Всего записей: ", len(used))
			return
		case <-ticker.C: //и, наконец, проверяем не пора ли вывести очередной отчет
			fmt.Printf("Всего %d / Повторов %d (%d записей/сек) \n", len(used), dupcount, quotescount/reportPeriod)
			quotescount = 0
		}
	}
}

func check(e error) {
	if e != nil {
		panic(e)
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

func readHashes() {
	//проверим файл на наличие
	if _, err := os.Stat(hashFile); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Файл хешей не найден, будет создан новый.")
			return
		}
	}

	fmt.Println("Чтение хешей...")
	hashfile, err := os.OpenFile(hashFile, os.O_RDONLY, 0666)
	check(err)
	defer hashfile.Close()
	//читать будем блоками по 16 байт - как раз один хеш:
	data := make([]byte, 16)
	for {
		n, err := hashfile.Read(data) //n вернет количество прочитанных байт, а err - ошибку, в случае таковой.
		if err != nil {
			if err == io.EOF {
				break
			}
			panic(err)
		}
		if n == 16 {
			used[hex.EncodeToString(data)] = true
		}
	}

	fmt.Println("Завершено. Прочитано хешей: ", len(used))
}
