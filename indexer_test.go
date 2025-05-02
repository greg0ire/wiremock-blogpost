package indexer

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/algolia/algoliasearch-client-go/v4/algolia/call"
	"github.com/algolia/algoliasearch-client-go/v4/algolia/debug"
	"github.com/algolia/algoliasearch-client-go/v4/algolia/search"
	"github.com/algolia/algoliasearch-client-go/v4/algolia/transport"
	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/wiremock/go-wiremock"
	testcontainers_wiremock "github.com/wiremock/wiremock-testcontainers-go"
)

const record = false

func spinUpContainer(t *testing.T) string {
	t.Helper()

	ctx := context.Background() // for some reason wiremock doesn't like the testing context

	absolutePath, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	var opts []testcontainers.ContainerCustomizer

	opts = append(opts, testcontainers_wiremock.WithImage("wiremock/wiremock:3.12.1"))

	if record {
		opts = append(opts, testcontainers.WithHostConfigModifier(func(hostConfig *container.HostConfig) {
			hostConfig.Binds = []string{
				absolutePath + "/testdata:/home/wiremock/mappings",
			}
		}))
	} else {
		mappingFiles, err := os.ReadDir(absolutePath + "/testdata")
		if err != nil {
			t.Fatalf("Failed to read testdata directory: %v", err)
		}
		for _, mappingFile := range mappingFiles {
			opts = append(opts, testcontainers_wiremock.WithMappingFile(
				mappingFile.Name(),
				"testdata/"+mappingFile.Name(),
			))
		}
	}

	container, err := testcontainers_wiremock.RunContainerAndStopOnCleanup(
		ctx,
		t,
		opts...,
	)

	if err != nil {
		t.Fatalf("Failed to create wiremock container: %v", err)
	}

	// The endpoint changes every time, so we need to obtain it at runtime
	host, err := container.Endpoint(ctx, "")

	if err != nil {
		t.Fatalf("Failed to get wiremock container endpoint: %v", err)
	}

	return host
}

func startRecording(t *testing.T, host string, appID string) {
	t.Helper()

	wiremockClient := wiremock.NewClient("http://" + host)
	t.Cleanup(func() {
		err := wiremockClient.Reset()
		if err != nil {
			t.Fatalf("Failed to reset wiremock: %v", err)
		}
	})

	if !record {
		return
	}
	debug.Enable()
	err := wiremockClient.StartRecording(fmt.Sprintf(
		"https://%s-dsn.algolia.net",
		appID,
	))

	if err != nil {
		t.Fatalf("Failed to start recording: %v", err)
	}

	t.Cleanup(func() {
		err := wiremockClient.StopRecording()
		if err != nil {
			t.Fatalf("Failed to stop recording: %v", err)
		}
	})
}

func newTestClient(t *testing.T, host, appID, apiKey string) *search.APIClient {
	t.Helper()

	algoliaClient, err := search.NewClientWithConfig(search.SearchConfiguration{
		Configuration: transport.Configuration{
			AppID:  appID,
			ApiKey: apiKey,
			Hosts: []transport.StatefulHost{
				transport.NewStatefulHost("http", host, func(k call.Kind) bool {
					return true
				}),
			},
		},
	})

	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	return algoliaClient
}

func TestIndexRecord(t *testing.T) {
	appID := os.Getenv("ALGOLIA_APP_ID")
	apiKey := os.Getenv("ALGOLIA_API_KEY")
	indexName := "test-index"

	host := spinUpContainer(t)

	startRecording(t, host, appID)

	algoliaClient := newTestClient(t, host, appID, apiKey)

	// Create index indirectly by setting settings
	_, err := algoliaClient.SetSettings(algoliaClient.NewApiSetSettingsRequest(
		"test-index",
		search.NewEmptyIndexSettings().SetSearchableAttributes([]string{"name"}),
	))

	if err != nil {
		t.Fatalf("Failed to set settings: %v", err)
	}

	t.Cleanup(func() {
		_, err := algoliaClient.DeleteIndex(algoliaClient.NewApiDeleteIndexRequest(indexName))
		if err != nil {
			t.Fatalf("Failed to delete index: %v", err)
		}
	})

	indexRecord(algoliaClient)

	// Search for 'test'
	searchResp, err := algoliaClient.SearchSingleIndex(
		algoliaClient.NewApiSearchSingleIndexRequest(indexName).WithSearchParams(
			search.SearchParamsObjectAsSearchParams(search.NewEmptySearchParamsObject().SetQuery("test")),
		),
	)

	if err != nil {
		panic(err)
	}

	if len(searchResp.Hits) == 0 {
		t.Fatal("No hits found")
	}

	firstHit := searchResp.Hits[0]

	if firstHit.AdditionalProperties["name"] != "test record" {
		t.Fatalf("Expected name to be 'test record', got '%s'", firstHit.AdditionalProperties["name"])
	}
}
