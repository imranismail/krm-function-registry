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
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestFilter(t *testing.T) {
	s := httptest.NewServer(registry.New())
	defer s.Close()

	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}

	createTestImages(t, u.Host)

	raw := createRawConfig(t, u.Host)
	deployment := createDeployment(t, u.Host)

	iu := NewImageUpdater()
	rl := framework.ResourceList{
		Items:          []*yaml.RNode{deployment},
		FunctionConfig: raw,
	}

	err = framework.LoadFunctionConfig(rl.FunctionConfig, &iu)
	assertNoError(t, err)

	err = iu.Process(&rl)
	assertNoError(t, err)

	if len(rl.Items) != 1 {
		t.Errorf("Expected 1, got '%d'", len(rl.Items))
	}

	image, err := rl.Items[0].Pipe(yaml.Lookup("spec", "template", "spec", "containers", "[name=test-container]", "image"))
	assertNoError(t, err)

	expected := fmt.Sprintf("%s/test/image:1.10.0", u.Host)
	if image.YNode().Value != expected {
		t.Errorf("Expected %s, got '%s'", expected, image.YNode().Value)
	}
}

func createTestImages(t *testing.T, host string) []string {
	images := []string{
		fmt.Sprintf("%s/test/image:1.0.0", host),
		fmt.Sprintf("%s/test/image:1.2.0", host),
		fmt.Sprintf("%s/test/image:1.10.0", host),
	}

	for _, img := range images {
		dst, _ := name.ParseReference(img)
		img, _ := random.Image(1024, 5)

		if err := remote.Write(dst, img); err != nil {
			t.Fatal(err)
		}
	}

	return images
}

func createRawConfig(t *testing.T, host string) *yaml.RNode {
	return yaml.MustParse(fmt.Sprintf(`
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
`, host))
}

func createDeployment(t *testing.T, host string) *yaml.RNode {
	return yaml.MustParse(fmt.Sprintf(`
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
`, host))
}

func assertNoError(t *testing.T, err error) {
	if err != nil {
		t.Errorf("Expected no error, got '%s'", err)
	}
}
