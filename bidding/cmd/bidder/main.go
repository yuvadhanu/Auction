package main

import (
	"bidding/api"
	"bidding/config"
	appmiddleware "bidding/middleware"
	"bidding/pkg/errors"
	"bidding/pkg/respond"
	"bidding/pkg/trace"
	"bidding/schema"
	"bytes"
	"encoding/json"
	er "errors"
	"flag"
	"log"
	"math/rand"
	"time"

	"fmt"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/rs/cors"
)

var (
	// Port in which the bidder client to run
	Port int
	// Delay after the bidder to respond to auction request
	Delay time.Duration
	// BidderID unique identifier of bidder
	BidderID string
)

func main() {
	name := flag.String("name", "Batman", "name identifier")
	port := flag.Int("port", 0, "port in which the bidder client to run")
	delay := flag.Uint("delay", 0, "delay after the bidder to respond to auction request")

	flag.Parse()

	if *port == 0 {
		log.Fatal("invalid port or port required")
	}
	if *delay > 500 {
		log.Println("delay more than 500ms 😟")
	}
	Port = *port
	Delay = (time.Duration(*delay) * time.Millisecond)

	trace.Setup(config.Env)
	router := chi.NewRouter()
	cors := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "OPTIONS", "DELETE"},
		AllowedHeaders: []string{
			"Origin", "Authorization", "Access-Control-Allow-Origin",
			"Access-Control-Allow-Header", "Accept",
			"Content-Type", "X-CSRF-Token",
		},
		ExposedHeaders: []string{
			"Content-Length", "Access-Control-Allow-Origin", "Origin",
		},
		AllowCredentials: true,
		MaxAge:           300,
	})

	// cross & loger middleware
	router.Use(cors.Handler)
	router.Use(
		middleware.Logger,
		appmiddleware.Recoverer,
	)

	router.Method(http.MethodPost, "/v1/bid", api.Handler(bidHandler))

	// register with auctioneer
	if err := registerWithAuctioneer(*name); err != nil {
		log.Fatal("Can't able to register with auctioneer. Err: ", err.Error())
	}

	trace.Log.Infof("Starting bidder %s on port :%d with delay %d\n",
		*name, *port, *delay)
	http.ListenAndServe(fmt.Sprintf(":%d", *port), router)
}

// NOTE: not doing anything with the request/auction_id
func bidHandler(w http.ResponseWriter, r *http.Request) *errors.AppError {
	<-time.After(Delay)
	rand.Seed(time.Now().UnixNano())
	min := 100
	max := 3000

	respond.OK(w, map[string]interface{}{
		"bidder_id": BidderID,
		"amount":    rand.Intn(max-min+1) + min,
	})
	return nil
}

func registerWithAuctioneer(name string) error {
	url := config.AuctioneerHost + "/v1/bidder/register"
	body := bytes.NewBuffer(nil)
	json.NewEncoder(body).Encode(map[string]interface{}{
		"name":  name,
		"delay": Delay,
	})

	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.Host = fmt.Sprintf("%s:%d", config.BidderHost, Port)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var res struct {
		Data *schema.Bidder `json:"data"`
		Meta respond.Meta   `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}
	if resp.StatusCode > 201 {
		return er.New(res.Meta.Message)
	}

	BidderID = res.Data.ID
	fmt.Println("Bidder ID: ", BidderID)
	return nil
}
