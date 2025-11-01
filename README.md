MarketStream: Real-Time Data Engine
MarketStream is a high-throughput, concurrent data pipeline built in Go, designed to simulate, process, and broadcast financial data in real-time. This project demonstrates a robust, backend-focused architecture, handling real-time WebSocket streaming, on-demand data aggregation from a NoSQL database, and a complete DevOps monitoring stack.

The application is fully containerized with Docker and observable via Prometheus and Grafana.

🚀 Core Features
Concurrent Go Backend: Utilizes goroutines and channels for a high-throughput, non-blocking pub/sub system to manage multiple data streams and clients.

Real-Time WebSocket Streaming: Broadcasts a simulated trade data stream ("ticks") via WebSockets to all subscribed clients.

Server-Side Aggregation: A concurrent service aggregates the raw trade stream into 5-second candlesticks (OHLCV/VWAP) in real-time and broadcasts them on a separate WebSocket topic.

Persistent Storage: Saves every raw trade to a MongoDB Atlas database for permanent storage.

On-Demand Historical API: A Go-powered REST API (/api/history-candles) that queries the MongoDB database, performs on-the-fly aggregation, and returns a complete JSON dataset of historical candlesticks.

Full DevOps & Monitoring Stack: Comes with a docker-compose.yml file to instantly launch the entire application, a Prometheus server, and a Grafana dashboard.

Live Metrics: Exposes a /metrics endpoint (e.g., market_simulator_connected_clients, market_simulator_trades_total) for live performance monitoring.

🛠️ Tech Stack
Backend: Go (Golang)

Database: MongoDB Atlas

Containerization: Docker & Docker Compose

Monitoring: Prometheus & Grafana

Real-time Comms: WebSockets

Frontend: HTML, CSS, JavaScript (for the dashboard)

🏗️ System Components
This project runs as a set of interconnected services, all managed by docker-compose.

market-app (The Go Service): This is the core application.

Trade Generator: A goroutine that simulates new trades every 500ms.

Broker: Manages all live WebSocket connections and client subscriptions.

Aggregator: A separate goroutine that builds 5-second candles from the live trade stream.

API Server: Serves the index.html file and the /api/history-candles REST endpoint.

mongodb (Database): While we connect to a cloud-hosted MongoDB Atlas, this is the persistent storage layer for all trade data.

prometheus: A time-series database that automatically scrapes the Go app's /metrics endpoint every 15 seconds.

grafana: A visualization dashboard that connects to Prometheus to display the application's health.

⚙️ Getting Started
Prerequisites

Docker and Docker Compose

Git

A free MongoDB Atlas account.

1. Set Up the Database

Create a free MongoDB Atlas cluster.

On the "Network Access" tab, "Allow Access From Anywhere" (0.0.0.0/0).

Get your connection string. It will look like mongodb+srv://user:pass@cluster...

Open the main.go file and paste your connection string into the connectToMongoDB function (around line 215).

2. Run the Full Stack

This project is designed to run with a single command.

Clone the repository:

Bash
git clone https://github.com/Laksharajjha/Marketsim
cd Marketsim
Ensure all files are correct: Make sure you have main.go, index.html, Dockerfile, docker-compose.yml, and prometheus.yml in the root directory.

Run go mod tidy:

Bash
go mod tidy
Build and run the stack:

Bash
docker-compose up --build
3. Access Your Services

Your entire system is now running:

Web Application: http://localhost:8080

Prometheus: http://localhost:9090

Grafana: http://localhost:3000 (login with admin / admin)

📈 How to Use
1. The Web App (:8080)

The web UI has three panels:

Live Ticks: Use the dropdown to subscribe to a raw trade stream (e.g., "AAPL").

5-Second Candlesticks (Live): Use the dropdown to subscribe to the live aggregated 5-second candle stream (e.g., "AAPL").

Historical Candle Chart: Select a ticker and click "Fetch Chart". This calls your Go API, which queries MongoDB and builds/returns a full historical chart.

2. The Grafana Dashboard (:3000)

Go to http://localhost:3000 (login admin/admin).

Add Prometheus as a data source (URL: http://prometheus:9090).

Create a new dashboard and add panels with these queries:

market_simulator_connected_clients (Shows active WebSocket users)

rate(market_simulator_trades_total[1m]) (Shows trades/sec)

3. API Endpoints

WS /ws: The main WebSocket endpoint for live data.

Client sends: {"action": "subscribe", "ticker": "AAPL"}

Client sends: {"action": "subscribe", "ticker": "5s-AAPL"}

GET /api/history-candles: The REST endpoint for historical data.

Query Param: ticker (e.g., ticker=AAPL)

Returns: A JSON array of all 5-second candle data from the database.
