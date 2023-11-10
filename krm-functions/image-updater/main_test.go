package main

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestFilter(t *testing.T) {
	s := httptest.NewServer(registry.New())
	defer s.Close()

	u, err := url.Parse(s.URL)

	if err != nil {
		t.Fatal(err)
	}

	images := []string{
		fmt.Sprintf("%s/test/image:1.0.0", u.Host),
		fmt.Sprintf("%s/test/image:1.2.0", u.Host),
		fmt.Sprintf("%s/test/image:1.10.0", u.Host),
	}

	for _, img := range images {
		dst, _ := name.ParseReference(img)
		img, _ := random.Image(1024, 5)

		if err := remote.Write(dst, img); err != nil {
			t.Fatal(err)
		}
	}

	raw := fmt.Sprintf(`
apiVersion: image-updater.imranismail.dev/v
kind: AutoUpdateImage
metadata:
  name: test
spec:
  targetImgSelector:
    pattern: ^%s/.+$
  remoteTagSelector:
    pattern: ^\d+\.(?P<minor>\d+)\.\d+$
    extract: minor
    sort: numerically
    order: desc
`, u.Host)

	testCase := []*yaml.RNode{
		yaml.MustParse(fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  template:
    spec:
      containers:
        - name: test-container
          image: %s/test/image:outdated
`, u.Host)),
	}

	api := AutoUpdateImage{}

	err = yaml.Unmarshal([]byte(raw), &api)
	if err != nil {
		t.Errorf("Expected no error, got '%s'", err)
	}

	result, err := filter(&api)(testCase)
	if err != nil {
		t.Errorf("Expected no error, got '%s'", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1, got '%d'", len(result))
	}

	image, err := result[0].Pipe(yaml.Lookup("spec", "template", "spec", "containers", "[name=test-container]", "image"))
	if err != nil {
		t.Errorf("Expected no error, got '%s'", err)
	}

	expected := fmt.Sprintf("%s/test/image:1.10.0", u.Host)
	if image.YNode().Value != expected {
		t.Errorf("Expected %s, got '%s'", expected, image.YNode().Value)
	}
}
