package main

import (
	"flag"
	"github.com/chfanghr/librespot"
	"github.com/chfanghr/librespot/Spotify"
	"github.com/chfanghr/librespot/utils"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

var (
	username  = flag.String("username", "", "username to login to spotify")
	password  = flag.String("password", "", "password to login to spotify")
	targetUri = flag.String("targetUri", "", "a valid spotify uri(a track or an album)")
)

func main() {
	flag.Parse()
	reg, err := regexp.Compile("^spotify:(track|album):[a-zA-Z0-9]+$")
	if err != nil {
		log.Fatalln(err)
	}
	if !reg.MatchString(*targetUri) {
		log.Fatalln("invalid spotify uri")
	}
	uriType := strings.Split(*targetUri, ":")[1]
	uri := strings.Split(*targetUri, ":")[2]

	if *password == "" || *username == "" {
		log.Fatalln("invalid username or password")
	}
	session, err := librespot.Login(*username, *password, "coverDownloader")
	if err != nil {
		log.Fatalln(err)
	}

	switch uriType {
	case "track":
		track, err := session.Mercury().GetTrack(utils.Base62ToHex(uri))
		if err != nil {
			log.Fatalln(err)
		}
		covers := utils.GetTrackCoverArtUrls(track)
		data := downloadCovers(covers)
		notOk, errs := saveCovers(track.GetName(), data)
		if notOk {
			log.Fatalln(errs)
		}
	case "album":
		album, err := session.Mercury().GetAlbum(utils.Base62ToHex(uri))
		if err != nil {
			log.Fatalln(err)
		}
		covers := utils.GetAlbumCoverArtUrls(album)
		data := downloadCovers(covers)
		notOk, errs := saveCovers(album.GetName(), data)
		if notOk {
			log.Fatalln(errs)
		}
	}
}
func downloadCovers(covers map[Spotify.Image_Size]string) map[Spotify.Image_Size][]byte {
	data := make(map[Spotify.Image_Size]chan []byte)
	wg := new(sync.WaitGroup)
	//producers
	for size, url := range covers {
		ch := make(chan []byte)
		data[size] = ch
		go func() {
			wg.Add(1)
			defer wg.Done()
			defer close(ch)
			res, err := new(http.Client).Get(url)
			if err != nil {
				return
			}
			if res.StatusCode != http.StatusOK {
				close(ch)
				return
			}
			defer func() { _ = res.Body.Close() }() //keep myself away from annoying warning
			buf, err := ioutil.ReadAll(res.Body)
			if err != nil {
				return
			}
			ch <- buf
			return
		}()
	}
	wg.Wait()
	//consumers
	Images := make(map[Spotify.Image_Size][]byte)

	for size, dataChan := range data {
		if data, ok := <-dataChan; ok {
			Images[size] = data
		}
	}
	return Images
}
func saveCovers(prefix string, data map[Spotify.Image_Size][]byte) (errorOccurred bool, errors []error) {
	wg := new(sync.WaitGroup)
	var errorsChan []chan error

	for size, image := range data {
		ch := make(chan error)
		errorsChan = append(errorsChan, ch)
		go func() {
			wg.Add(1)
			defer wg.Done()
			defer close(ch)
			contentType := http.DetectContentType(image)
			typeInfo := strings.Split(contentType, "/")
			var suffix string
			if len(typeInfo) == 0 {
				suffix = ""
			} else if len(typeInfo) == 1 || (len(typeInfo) > 2 && typeInfo[0] != "image") {
				suffix = "." + contentType
			} else {
				suffix = "." + typeInfo[1]
			}
			err := ioutil.WriteFile(prefix+"-"+size.String()+suffix, image, 0666)
			if err != nil {
				ch <- err
			}
		}()
	}
	wg.Wait()
	for _, ch := range errorsChan {
		if err, ok := <-ch; ok {
			errors = append(errors, err)
		}
	}
	if len(errors) != 0 {
		errorOccurred = true
	}
	return
}
