package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

//go:embed index.html
var htmlPage []byte

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	id := &IngressDirectory{
		clientset: clientset,
	}
	t := time.NewTicker(5 * time.Second)

	var wg sync.WaitGroup
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		_, err := w.Write(htmlPage)
		if err != nil {
			log.Printf("error: serving html: %v", err)
		}
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, req *http.Request) {
		if err := json.NewEncoder(w).Encode(id); err != nil {
			log.Printf("error: generating json: %v", err)
		}
	})
	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	wg.Add(1)
	go func() {
		log.Fatal(srv.ListenAndServe())
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				t.Stop()
				timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Shutdown(timeout); err != nil {
					log.Printf("error: shutting down http server: %v", err)
				}
				break
			case <-t.C:
				id.Update(ctx)
			}
		}
		wg.Done()
	}()

	wg.Wait()

	return nil
}

type IngressDirectory struct {
	Targets     []IngressDirectoryTarget `json:"targets"`
	LastUpdated int64                    `json:"last_updated"`

	clientset *kubernetes.Clientset
}

func (id *IngressDirectory) Update(ctx context.Context) {
	ingresses, err := id.clientset.NetworkingV1().Ingresses("").List(ctx, meta.ListOptions{})
	if err != nil {
		log.Printf("error: listing ingress: %v", err)
	} else {
		targets := []IngressDirectoryTarget{}
		for _, t := range ingresses.Items {
			namespace := t.Namespace
			name := t.Name
			for _, r := range t.Spec.Rules {
				host := r.Host
				for _, p := range r.HTTP.Paths {
					path := p.Path
					targets = append(targets, IngressDirectoryTarget{
						Namespace: namespace,
						Name:      name,
						Host:      host,
						Path:      path,
					})
				}
			}
		}
		id.Targets = targets
		id.LastUpdated = time.Now().Unix()
	}
}

type IngressDirectoryTarget struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Path      string `json:"path,omitempty"`
}
