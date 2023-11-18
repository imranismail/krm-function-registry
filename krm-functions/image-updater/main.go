package main

import (
	"errors"
	"io"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"

	ecr "github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	acr "github.com/chrismellard/docker-credential-acr-env/pkg/credhelper"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/fn/framework/command"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type ImageUpdater struct {
	metav1.GroupVersionKind `yaml:",inline" json:",inline"`
	Metadata                *metav1.ObjectMeta `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	Spec                    *ImageUpdaterSpec  `json:"spec,omitempty" yaml:"spec,omitempty"`
}

type ImageUpdaterSpec struct {
	TargetImgSelector *TargetImgSelector `json:"targetImgSelector,omitempty" yaml:"targetImgSelector,omitempty"`
	RemoteTagSelector *RemoteTagSelector `json:"remoteTagSelector,omitempty" yaml:"remoteTagSelector,omitempty"`
}

type PatternSelector struct {
	Pattern *regexp.Regexp `json:"pattern,omitempty" yaml:"pattern,omitempty"`
}

type TargetImgSelector struct {
	PatternSelector `json:",inline" yaml:",inline"`
}

type RemoteTagSelector struct {
	PatternSelector `json:",inline" yaml:",inline"`
	Extract         string `json:"extract,omitempty" yaml:"extract,omitempty"`
	Sort            string `json:"sort,omitempty" yaml:"sort,omitempty"`
	Order           string `json:"order,omitempty" yaml:"order,omitempty"`
}

// NewImageUpdater creates a new instance of ImageUpdater.
func NewImageUpdater() ImageUpdater {
	return ImageUpdater{}
}

// Default sets default values for ImageUpdater if they are not set.
func (iu *ImageUpdater) Default() error {
	if iu.Spec.RemoteTagSelector.Sort == "" {
		iu.Spec.RemoteTagSelector.Sort = "alphabetically"
	}

	if iu.Spec.RemoteTagSelector.Order == "" {
		iu.Spec.RemoteTagSelector.Order = "asc"
	}

	if iu.Spec.TargetImgSelector.Pattern == nil {
		iu.Spec.TargetImgSelector.Pattern = regexp.MustCompile(".*")
	}

	if iu.Spec.RemoteTagSelector.Pattern == nil {
		iu.Spec.RemoteTagSelector.Pattern = regexp.MustCompile(".*")
	}

	return nil
}

// Validate checks if the values of ImageUpdater are valid.
func (iu *ImageUpdater) Validate() error {
	if iu.Spec.RemoteTagSelector.Sort != "alphabetically" && iu.Spec.RemoteTagSelector.Sort != "numerically" {
		return errors.New("spec.remoteTagSelector.sort must be either 'alphabetically' or 'numerically'")
	}

	if iu.Spec.RemoteTagSelector.Order != "asc" && iu.Spec.RemoteTagSelector.Order != "desc" {
		return errors.New("spec.remoteTagSelector.order must be either 'asc' or 'desc'")
	}

	return nil
}

// Process processes the ResourceList in ImageUpdater.
func (iu *ImageUpdater) Process(rl *framework.ResourceList) error {
	if err := framework.LoadFunctionConfig(rl.FunctionConfig, iu); err != nil {
		return err
	}

	keychain := iu.createKeychain()

	for _, item := range rl.Items {
		err := iu.processItem(item, keychain)
		if err != nil {
			return err
		}
	}

	return nil
}

// createKeychain creates a new keychain for authentication.
func (iu *ImageUpdater) createKeychain() remote.Option {
	return remote.WithAuthFromKeychain(
		authn.NewMultiKeychain(
			authn.DefaultKeychain,
			authn.NewKeychainFromHelper(
				ecr.NewECRHelper(
					ecr.WithLogger(io.Discard),
				),
			),
			google.Keychain,
			authn.NewKeychainFromHelper(
				acr.NewACRCredentialsHelper(),
			),
		),
	)
}

// processItem processes a single item in the ResourceList.
func (iu *ImageUpdater) processItem(item *yaml.RNode, keychain remote.Option) error {
	containers, err := iu.getContainers(item)
	if err != nil {
		return err
	}

	return containers.VisitElements(iu.processContainer(item, keychain))
}

// getContainers retrieves the containers from a Kubernetes resource.
func (iu *ImageUpdater) getContainers(item *yaml.RNode) (*yaml.RNode, error) {
	switch item.GetKind() {
	case "Deployment", "StatefulSet", "DaemonSet", "Job":
		return item.Pipe(yaml.Lookup("spec", "template", "spec", "containers"))
	case "CronJob":
		return item.Pipe(yaml.Lookup("spec", "jobTemplate", "spec", "template", "spec", "containers"))
	default:
		return nil, nil
	}
}

// processContainer processes a single container in a Kubernetes resource.
func (iu *ImageUpdater) processContainer(item *yaml.RNode, keychain remote.Option) func(node *yaml.RNode) error {
	return func(node *yaml.RNode) error {
		image, err := node.GetString("image")
		if err != nil {
			return err
		}

		ref, err := name.ParseReference(image)
		if err != nil {
			return err
		}

		if !iu.Spec.TargetImgSelector.Pattern.MatchString(ref.String()) {
			return nil
		}

		itemId := iu.getItemId(item)

		log.Printf("processing %s: %s", itemId, ref.String())

		images, err := remote.List(ref.Context(), keychain)
		if err != nil {
			return err
		}

		filtered := iu.filterImages(images)

		iu.sortImages(filtered)

		if len(filtered) > 0 {
			return iu.updateImage(node, ref, filtered[0], itemId)
		}

		return nil
	}
}

// getItemId generates an ID for a Kubernetes resource.
func (iu *ImageUpdater) getItemId(item *yaml.RNode) string {
	namespace := item.GetNamespace()

	if namespace == "" {
		namespace = "-"
	}

	return item.GetKind() + "/" + namespace + "/" + item.GetName()
}

// filterImages filters out images based on the RemoteTagSelector pattern.
func (iu *ImageUpdater) filterImages(images []string) []string {
	filtered := []string{}
	for _, image := range images {
		if iu.Spec.RemoteTagSelector.Pattern.MatchString(image) {
			filtered = append(filtered, image)
		}
	}
	return filtered
}

// sortImages sorts images based on the RemoteTagSelector sort and order
func (iu *ImageUpdater) sortImages(images []string) {
	cmp := iu.Spec.RemoteTagSelector.Pattern.SubexpIndex(iu.Spec.RemoteTagSelector.Extract)

	sort.Slice(images, func(i, j int) bool {
		var (
			left  string
			right string
		)

		if cmp == -1 {
			left = images[i]
			right = images[j]
		} else {
			left = iu.Spec.RemoteTagSelector.Pattern.FindStringSubmatch(images[i])[cmp]
			right = iu.Spec.RemoteTagSelector.Pattern.FindStringSubmatch(images[j])[cmp]
		}

		if iu.Spec.RemoteTagSelector.Sort == "numerically" {
			leftInt := mustAToI(left)
			rightInt := mustAToI(right)

			if iu.Spec.RemoteTagSelector.Order == "asc" {
				return leftInt < rightInt
			} else {
				return leftInt > rightInt
			}
		} else {
			if iu.Spec.RemoteTagSelector.Order == "asc" {
				return left < right
			} else {
				return left > right
			}
		}
	})
}

// updateImage updates the image reference in a Kubernetes resource
func (iu *ImageUpdater) updateImage(node *yaml.RNode, ref name.Reference, image string, itemId string) error {
	next := ref.Context().Tag(image).String()
	log.Printf("processed %s: %s -> %s", itemId, ref.String(), next)

	return node.PipeE(yaml.SetField("image", yaml.NewStringRNode(next)))
}

// mustAToI converts a string to an integer and panics if there is an error.
func mustAToI(s string) int {
	i, err := strconv.Atoi(s)

	if err != nil {
		log.Fatal(err)
	}

	return i
}

func init() {
	log.SetOutput(os.Stderr)
}

func main() {
	iu := NewImageUpdater()
	cmd := command.Build(&iu, command.StandaloneEnabled, false)
	command.AddGenerateDockerfile(cmd)

	if err := cmd.Execute(); err != nil {
		log.Fatalf("error: %v", err)
		os.Exit(1)
	}
}
