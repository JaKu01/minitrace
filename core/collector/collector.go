package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/JaKu01/minitrace/core/internal/api"
	"github.com/JaKu01/minitrace/core/internal/trace"
)

type Collector struct {
	Urls []string
}

type CollectionResult struct {
	url      string
	response []api.SpanDTO
}

func NewCollector(urls []string) (*Collector, error) {
	trace.EnsureServerStarted()
	return &Collector{
		Urls: urls,
	}, nil
}

func (c *Collector) CollectTraces() []CollectionResult {
	var wg sync.WaitGroup
	results := make(chan CollectionResult, len(c.Urls))

	for _, parsedUrl := range c.Urls {

		wg.Add(1)
		go func(parsedUrl string) {
			defer wg.Done()
			resp, err := collectAtUrl(parsedUrl)
			if err != nil {
				fmt.Printf("Error: %v", err)
			}
			results <- CollectionResult{
				url:      parsedUrl,
				response: resp,
			}
		}(parsedUrl)
	}
	wg.Wait()
	close(results)

	var collectionResults []CollectionResult
	for result := range results {
		collectionResults = append(collectionResults, result)
	}
	return collectionResults
}

func collectAtUrl(url string) ([]api.SpanDTO, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var collectionResponseBody []api.SpanDTO
	err = json.NewDecoder(resp.Body).Decode(&collectionResponseBody)
	return collectionResponseBody, err
}
