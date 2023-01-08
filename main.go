package main

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

//go:embed serve/index.html serve/*.css
var staticFS embed.FS
var devMode = os.Getenv("DEV_MODE") != ""

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	var config *rest.Config
	if devMode {
		home, err := os.UserHomeDir()
		if err != nil {
			panic(err.Error())
		}
		c, err := clientcmd.BuildConfigFromFlags("", filepath.Join(home, ".kube", "config"))
		if err != nil {
			panic(err.Error())
		}
		config = c
	} else {
		c, err := rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
		config = c
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
	mux.HandleFunc("/api", func(w http.ResponseWriter, req *http.Request) {
		log.Printf("/api")
		if err := json.NewEncoder(w).Encode(id); err != nil {
			log.Printf("error: generating json: %v", err)
		}
	})
	var serveFS http.FileSystem
	if devMode {
		serveFS = http.Dir("./serve")
	} else {
		sub, err := fs.Sub(staticFS, "serve")
		if err != nil {
			panic(err.Error())
		}
		serveFS = http.FS(sub)
	}
	mux.Handle("/", http.FileServer(serveFS))
	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	wg.Add(1)
	go func() {
		id.Update(ctx)
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
	LastUpdated string                   `json:"last_updated"`

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
		id.LastUpdated = time.Now().In(time.UTC).Format(time.RFC3339)
	}
}

type IngressDirectoryTarget struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Path      string `json:"path,omitempty"`
}
