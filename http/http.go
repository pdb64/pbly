package http

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/felixge/httpsnoop"
	"github.com/p16n/pbdb/db"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
)

var (
	requestsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pbly_requests",
			Help: "The total number of requests",
		},
		[]string{"url", "method", "status"},
	)

	requestDurationGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pbly_request_duration",
			Help: "The duration of each request in microseconds (µs)",
		},
		[]string{"url", "method", "status"},
	)
)

func rootHandler(w http.ResponseWriter, r *http.Request) {
	k := r.URL.Path[len("/"):]

	if k == "" {
		fmt.Fprint(w, "pbly is a link shortener")
		return
	}

	b, err := db.Get(k)
	if err != nil {
		log.Printf("Err: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if len(b) < 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	http.Redirect(w, r, string(b), http.StatusSeeOther)
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	token := viper.GetString("token")
	if r.Header.Get("Pbly-Token") != token {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	k := r.URL.Path[len("/new/"):]
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Err: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = db.Set(k, b)
	if err != nil {
		log.Printf("Err: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func Serve() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/new/", postHandler)
	mux.Handle("/metrics/", promhttp.Handler())

	wrappedMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := httpsnoop.CaptureMetrics(mux, w, r)
		log.Printf(
			"%s %s (code=%d dt=%s written=%d)",
			r.Method,
			r.URL,
			m.Code,
			m.Duration,
			m.Written,
		)

		requestsCounter.WithLabelValues(
			fmt.Sprintf("%s", r.URL),
			fmt.Sprintf("%s", r.Method),
			fmt.Sprintf("%d", m.Code),
		).Inc()

		requestDurationGauge.WithLabelValues(
			fmt.Sprintf("%s", r.URL),
			fmt.Sprintf("%s", r.Method),
			fmt.Sprintf("%d", m.Code),
		).Set(float64(m.Duration))
	})

	port := fmt.Sprintf(":%s", viper.GetString("port"))

	log.Printf("Starting pbly server on port %s\n", port)
	log.Fatal(http.ListenAndServe(port, wrappedMux))
}
