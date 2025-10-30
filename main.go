package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os" // NEW: Import the OS package
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// --- STRUCTS (Unchanged) ---
type Trade struct {
	ID     primitive.ObjectID `json:"-" bson:"_id,omitempty"`
	Ticker string             `json:"ticker" bson:"ticker"`
	Price  float64            `json:"price" bson:"price"`
	Volume int                `json:"volume" bson:"volume"`
	Time   time.Time          `json:"timestamp" bson:"timestamp"`
}
type ClientMessage struct {
	Action string `json:"action"`
	Ticker string `json:"ticker"`
}
type Candlestick struct {
	Ticker    string    `json:"ticker"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    int       `json:"volume"`
	VWAP      float64   `json:"vwap"`
	StartTime time.Time `json:"start_time"`
}
type aggBucket struct {
	open    float64
	high    float64
	low     float64
	volume  int
	totalPV float64
	close   float64
	isSet   bool
}

// --- FAKE TRADE GENERATOR (Unchanged) ---
var tickers = []string{"AAPL", "GOOGL", "MSFT", "TSLA", "GOLANG"}
func generateFakeTrade() Trade {
	ticker := tickers[rand.Intn(len(tickers))]
	basePrices := map[string]float64{
		"AAPL": 150.0, "GOOGL": 2800.0, "MSFT": 300.0, "TSLA": 750.0, "GOLANG": 100.0,
	}
	basePrice := basePrices[ticker]
	price := basePrice + (rand.Float64()-0.5)*(basePrice*0.05)
	return Trade{
		Ticker: ticker, Price: price, Volume: rand.Intn(100) + 1, Time: time.Now(),
	}
}

// --- PROMETHEUS (Unchanged) ---
var (
	connectedClients = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "market_simulator_connected_clients",
		Help: "The current number of connected WebSocket clients.",
	})
	tradesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "market_simulator_trades_total",
		Help: "The total number of trades generated.",
	})
)

// --- BROKER (Unchanged) ---
type Broker struct {
	subscriptions map[string]map[*websocket.Conn]bool
	clientSubs    map[*websocket.Conn]map[string]bool
	register      chan *websocket.Conn
	unregister    chan *websocket.Conn
	broadcastTrade  chan Trade
	broadcastCandle chan Candlestick
	clientMessages chan struct {
		conn    *websocket.Conn
		message []byte
	}
	tradesCollection *mongo.Collection
	mu               sync.Mutex
}
func (b *Broker) run() {
	for {
		select {
		case conn := <-b.register:
			b.mu.Lock()
			b.clientSubs[conn] = make(map[string]bool)
			log.Println("Live Client registered. Total clients:", len(b.clientSubs))
			connectedClients.Set(float64(len(b.clientSubs)))
			b.mu.Unlock()
		case conn := <-b.unregister:
			b.mu.Lock()
			if subs, ok := b.clientSubs[conn]; ok {
				for ticker := range subs {
					delete(b.subscriptions[ticker], conn)
				}
				delete(b.clientSubs, conn)
			}
			conn.Close()
			log.Println("Live Client unregistered. Total clients:", len(b.clientSubs))
			connectedClients.Set(float64(len(b.clientSubs)))
			b.mu.Unlock()
		case msg := <-b.clientMessages:
			b.handleClientMessage(msg.conn, msg.message)
		case trade := <-b.broadcastTrade:
			go b.saveTrade(trade)
			b.mu.Lock()
			if clients, ok := b.subscriptions[trade.Ticker]; ok {
				for client := range clients {
					if err := client.WriteJSON(trade); err != nil {
						log.Println("WriteJSON tick error:", err)
						b.unregister <- client
					}
				}
			}
			b.mu.Unlock()
		case candle := <-b.broadcastCandle:
			b.mu.Lock()
			topic := fmt.Sprintf("5s-%s", candle.Ticker)
			if clients, ok := b.subscriptions[topic]; ok {
				for client := range clients {
					if err := client.WriteJSON(candle); err != nil {
						log.Println("WriteJSON candle error:", err)
						b.unregister <- client
					}
				}
			}
			b.mu.Unlock()
		}
	}
}
func (b *Broker) handleClientMessage(conn *websocket.Conn, message []byte) {
	var msg ClientMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Println("Error parsing client message:", err)
		return
	}
	topic := msg.Ticker
	b.mu.Lock()
	defer b.mu.Unlock()
	switch msg.Action {
	case "subscribe":
		if _, ok := b.subscriptions[topic]; !ok {
			b.subscriptions[topic] = make(map[*websocket.Conn]bool)
		}
		b.subscriptions[topic][conn] = true
		b.clientSubs[conn][topic] = true
		log.Printf("Client subscribed to %s", topic)
	case "unsubscribe":
		if subs, ok := b.subscriptions[topic]; ok {
			delete(subs, conn)
		}
		if subs, ok := b.clientSubs[conn]; ok {
			delete(subs, topic)
		}
		log.Printf("Client unsubscribed from %s", topic)
	}
}
func (b *Broker) saveTrade(trade Trade) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := b.tradesCollection.InsertOne(ctx, trade)
	if err != nil {
		log.Println("Error saving trade to MongoDB:", err)
		return
	}
	tradesTotal.Inc()
}
func connectToMongoDB() *mongo.Client {
	connStr := "mongodb+srv://laksharajjha_db_user:ipsh9sQoukdvDFGD@marketv01.mdfkckq.mongodb.net/?appName=MarketV01"
	clientOptions := options.Client().ApplyURI(connStr)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil { log.Fatal(err) }
	err = client.Ping(ctx, nil)
	if err != nil { log.Fatal("Could not ping MongoDB:", err) }
	log.Println("Successfully connected to MongoDB Atlas!")
	return client
}
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}
func newBroker(tradesCollection *mongo.Collection) *Broker {
	return &Broker{
		subscriptions:    make(map[string]map[*websocket.Conn]bool),
		clientSubs:       make(map[*websocket.Conn]map[string]bool),
		register:         make(chan *websocket.Conn),
		unregister:       make(chan *websocket.Conn),
		broadcastTrade:   make(chan Trade),
		broadcastCandle:  make(chan Candlestick),
		clientMessages:   make(chan struct { conn *websocket.Conn; message []byte }),
		tradesCollection: tradesCollection,
	}
}

// --- AGGREGATOR (Unchanged) ---
type Aggregator struct {
	tradeChan chan Trade
	broker    *Broker
	buckets   map[string]*aggBucket
	mu        sync.Mutex
	window    time.Duration
}
func newAggregator(broker *Broker, window time.Duration) *Aggregator {
	return &Aggregator{
		tradeChan: make(chan Trade, 1000),
		broker:    broker,
		buckets:   make(map[string]*aggBucket),
		window:    window,
	}
}
func (a *Aggregator) run() {
	ticker := time.NewTicker(a.window)
	defer ticker.Stop()
	for {
		select {
		case trade := <-a.tradeChan:
			a.mu.Lock()
			if _, ok := a.buckets[trade.Ticker]; !ok {
				a.buckets[trade.Ticker] = &aggBucket{}
			}
			bucket := a.buckets[trade.Ticker]
			if !bucket.isSet {
				bucket.open = trade.Price
				bucket.high = trade.Price
				bucket.low = trade.Price
				bucket.close = trade.Price
				bucket.volume = trade.Volume
				bucket.totalPV = trade.Price * float64(trade.Volume)
				bucket.isSet = true
			} else {
				if trade.Price > bucket.high { bucket.high = trade.Price }
				if trade.Price < bucket.low { bucket.low = trade.Price }
				bucket.close = trade.Price
				bucket.volume += trade.Volume
				bucket.totalPV += trade.Price * float64(trade.Volume)
			}
			a.mu.Unlock()
		case <-ticker.C:
			a.mu.Lock()
			startTime := time.Now().Add(-a.window).Truncate(a.window)
			for ticker, bucket := range a.buckets {
				if !bucket.isSet { continue }
				var vwap float64
				if bucket.volume > 0 { vwap = bucket.totalPV / float64(bucket.volume) }
				candle := Candlestick{
					Ticker: ticker, Open: bucket.open, High: bucket.high, Low: bucket.low,
					Close: bucket.close, Volume: bucket.volume, VWAP: vwap, StartTime: startTime,
				}
				a.broker.broadcastCandle <- candle
			}
			a.buckets = make(map[string]*aggBucket)
			a.mu.Unlock()
		}
	}
}

// --- HISTORICAL CANDLE API (Unchanged) ---
func handleCandleHistory(tradesCollection *mongo.Collection) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ticker := r.URL.Query().Get("ticker")
		if ticker == "" {
			http.Error(w, "Missing 'ticker' query parameter", http.StatusBadRequest)
			return
		}
		window := 5 * time.Second
		filter := bson.M{"ticker": ticker}
		opts := options.Find().SetSort(bson.D{{"timestamp", 1}})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cursor, err := tradesCollection.Find(ctx, filter, opts)
		if err != nil {
			http.Error(w, "Database query failed", http.StatusInternalServerError)
			log.Println("Error querying history:", err)
			return
		}
		defer cursor.Close(ctx)
		var candles []Candlestick
		var currentBucket *aggBucket
		var currentWindowStart time.Time
		for cursor.Next(ctx) {
			var trade Trade
			if err := cursor.Decode(&trade); err != nil {
				log.Println("Error decoding trade for history:", err)
				continue
			}
			if currentBucket == nil {
				currentBucket = &aggBucket{
					open:    trade.Price, high:  trade.Price, low:   trade.Price, close: trade.Price,
					volume:  trade.Volume, totalPV: trade.Price * float64(trade.Volume), isSet: true,
				}
				currentWindowStart = trade.Time.Truncate(window)
				continue
			}
			if !trade.Time.Before(currentWindowStart.Add(window)) {
				var vwap float64
				if currentBucket.volume > 0 {
					vwap = currentBucket.totalPV / float64(currentBucket.volume)
				}
				candles = append(candles, Candlestick{
					Ticker:    ticker, Open: currentBucket.open, High: currentBucket.high, Low: currentBucket.low,
					Close: currentBucket.close, Volume: currentBucket.volume, VWAP: vwap, StartTime: currentWindowStart,
				})
				currentBucket = &aggBucket{
					open:    trade.Price, high:  trade.Price, low:   trade.Price, close: trade.Price,
					volume:  trade.Volume, totalPV: trade.Price * float64(trade.Volume), isSet: true,
				}
				currentWindowStart = trade.Time.Truncate(window)
			} else {
				if trade.Price > currentBucket.high { currentBucket.high = trade.Price }
				if trade.Price < currentBucket.low  { currentBucket.low = trade.Price }
				currentBucket.close = trade.Price
				currentBucket.volume += trade.Volume
				currentBucket.totalPV += trade.Price * float64(trade.Volume)
			}
		}
		if currentBucket != nil && currentBucket.isSet {
			var vwap float64
			if currentBucket.volume > 0 {
				vwap = currentBucket.totalPV / float64(currentBucket.volume)
			}
			candles = append(candles, Candlestick{
				Ticker:    ticker, Open: currentBucket.open, High: currentBucket.high, Low: currentBucket.low,
				Close: currentBucket.close, Volume: currentBucket.volume, VWAP: vwap, StartTime: currentWindowStart,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(candles)
	}
}


// --- MAIN FUNCTION (UPDATED) ---
func main() {
	client := connectToMongoDB()
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			log.Println("Error disconnecting from MongoDB:", err)
		}
	}()
	tradesCollection := client.Database("marketdata").Collection("trades")

	broker := newBroker(tradesCollection)
	go broker.run()

	agg := newAggregator(broker, 5*time.Second)
	go agg.run()

	// --- FAKE GENERATOR RE-ADDED ---
	go func() {
		for {
			trade := generateFakeTrade()
			broker.broadcastTrade <- trade
			agg.tradeChan <- trade
			time.Sleep(500 * time.Millisecond)
		}
	}()

	// --- HTTP Handlers ---
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	
	http.HandleFunc("/api/history-candles", handleCandleHistory(tradesCollection))

	http.HandleFunc("/ws", func(w http.SERVICE_UNAVAILABLE, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Error upgrading:", err)
			return
		}
		broker.register <- conn
		go func() {
			defer func() { broker.unregister <- conn }()
			for {
				msgType, msg, err := conn.ReadMessage()
				if err != nil { break }
				if msgType == websocket.TextMessage {
					broker.clientMessages <- struct {
						conn    *websocket.Conn
						message []byte
					}{conn, msg}
				}
			}
		}()
	})

	http.Handle("/metrics", promhttp.Handler())

	// --- THIS IS THE PORT FIX ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default for local
	}
	
	fmt.Println("Starting server on :", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
	// --- END OF FIX ---
}