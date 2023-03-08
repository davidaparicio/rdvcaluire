package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
	const backoff = 5 * time.Second
	const want = http.StatusNotFound
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

	// ../../../../go/pkg/mod/github.com/hajimehoshi/oto@v1.0.1/context.go:69:12: undefined: newDriver
	// To fix this error, we need to enable CGO_ENABLED=1
	err = speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
	if err != nil {
		log.Fatal(err)
	}

	//https://github.com/faiface/beep/wiki/To-buffer,-or-not-to-buffer,-that-is-the-question
	buffer := beep.NewBuffer(format)
	buffer.Append(streamer)
	err = streamer.Close()
	if err != nil {
		log.Fatal(err)
	}

	// Graceful shutdown goroutine
	go func(context.CancelFunc) {
		sigquit := make(chan os.Signal, 1)
		// os.Kill can't be caught https://groups.google.com/g/golang-nuts/c/t2u-RkKbJdU
		// POSIX spec: signal can be caught except SIGKILL/SIGSTOP signals
		// Ctrl-c (usually) sends the SIGINT signal, not SIGKILL
		// syscall.SIGTERM usual signal for termination
		// and default one for docker containers, which is also used by kubernetes
		signal.Notify(sigquit, os.Interrupt, os.Kill, syscall.SIGTERM)
		sig := <-sigquit

		sugar.Infow("Caught the following signal", "signal", sig)
		cancel()
	}(cancel)

	ticker := time.NewTicker(backoff)
	for {
		select {
		case <-ticker.C:
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
			if resp.StatusCode == want {
				sugar.Infow("Check",
					"attempt", attempt,
					"statuscode", resp.StatusCode,
					"backoff", backoff,
					"url", url,
				)
				music := buffer.Streamer(0, buffer.Len())
				speaker.Play(music)
				// The check loop will continue (and the music can re-run), until the cancellation by the user
			} else {
				sugar.Infow("unmatch status code",
					"attempt", attempt,
					"statuscode", resp.StatusCode,
					"backoff", backoff,
					"url", url,
				)
			}
			attempt++
		case <-ctx.Done():
			sugar.Infow("Graceful shutdown...", ctx.Err().Error())
			return
		}
	}
}
