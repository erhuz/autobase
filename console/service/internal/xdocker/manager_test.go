package xdocker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/docker/docker/client"
	"github.com/rs/zerolog"
)

func TestV61AutomationLifecycleUsesDockerNamesAndCIDLessCleanup(t *testing.T) {
	var created atomic.Int32
	var removed atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images/"):
			_, _ = w.Write([]byte(`{"Id":"sha256:test"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/containers/create"):
			if name := r.URL.Query().Get("name"); name != "" {
				http.Error(w, "container name must be empty", http.StatusBadRequest)
				return
			}
			id := created.Add(1)
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"Id":"container-%d","Warnings":[]}`, id)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/containers/container-"):
			removed.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	transport, err := NewRoundTripperLog(server.URL, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	dockerClient, err := client.NewClientWithOpts(
		client.WithHost(server.URL),
		client.WithHTTPClient(&http.Client{Transport: transport}),
		client.WithVersion("1.47"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer dockerClient.Close()
	manager := &dockerManager{cli: dockerClient, log: zerolog.Nop(), image: "test-image"}

	var ids [2]InstanceID
	var manageErrs [2]error
	var wait sync.WaitGroup
	for i := range ids {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ids[i], manageErrs[i] = manager.ManageCluster(context.Background(), &ManageClusterConfig{})
		}()
	}
	wait.Wait()

	for i, manageErr := range manageErrs {
		if manageErr != nil {
			t.Fatalf("ManageCluster(%d): %v", i, manageErr)
		}
		if err = manager.RemoveContainer(context.Background(), ids[i]); err != nil {
			t.Fatalf("RemoveContainer(%d): %v", i, err)
		}
	}
	if ids[0] == ids[1] {
		t.Fatalf("container IDs = %q, %q", ids[0], ids[1])
	}
	if created.Load() != 2 || removed.Load() != 2 {
		t.Fatalf("created=%d removed=%d", created.Load(), removed.Load())
	}
}
