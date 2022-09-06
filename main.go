package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"go.uber.org/zap"
)

func main() {
	// http://rdv.ville-caluire.fr/eAppointment/element/jsp/appointment.jsp
	const url = "https://rdv.ville-caluire.fr/eAppointment/appointment.do" //https://ville-caluire.toodego.com/page-information-etat-civil/
	const musicFile = "You_Suffer_by_Napalm_Death.mp3"                     // got from https://archive.org/details/YouSufferNapalmDeath
	const backoff = 30 * time.Second
	var attempt int = 1

	//ctx := context.TODO()
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	client := &http.Client{}

	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	sugar := logger.Sugar()

	f, err := os.Open(musicFile)
	if err != nil {
		log.Fatal(err)
	}

	streamer, format, err := mp3.Decode(f)
	if err != nil {
		log.Fatal(err)
	}
	defer streamer.Close()

	speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))

	ticker := time.NewTicker(backoff)
	done := make(chan bool)
	for {
		select {
		case <-ticker.C: // select case must be send or receive
			//fmt.Printf("Check %d\n", attempt)
			req, err1 := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				fmt.Println(err1)
				return
			}
			resp, err2 := client.Do(req)
			if err != nil {
				fmt.Println(err2)
				return
			}
			//fmt.Println(resp.StatusCode)
			if resp.StatusCode == http.StatusNotFound {
				logger.Info("Play music")
				speaker.Play(beep.Seq(streamer, beep.Callback(func() {
					done <- true
				})))
				<-done
				logger.Info("Music end")
				// The loop will continue (and the music can re-run), until the cancellation by the user
			} else {
				sugar.Infow("failed to fetch URL",
					// Structured context as loosely typed key-value pairs.
					"url", url,
					"attempt", attempt,
					"statuscode", resp.StatusCode,
					"backoff", 30*time.Second,
				)
				//sugar.Infof("Failed to fetch URL: %s", url)
			}
			attempt++
		case <-ctx.Done():
			// If the request gets cancelled, log it to STDERR
			//fmt.Fprint(os.Stderr, "request cancelled\n")
			logger.Error("request cancelled\n")
			cancel()
			return
		}
	}
}
