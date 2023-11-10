package main

import (
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
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type AutoUpdateImage struct {
	metav1.GroupVersionKind `yaml:",inline" json:",inline"`
	Metadata                *metav1.ObjectMeta   `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	Spec                    *AutoUpdateImageSpec `json:"spec,omitempty" yaml:"spec,omitempty"`
}

type AutoUpdateImageSpec struct {
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

func NewAutoUpdateImage() AutoUpdateImage {
	return AutoUpdateImage{
		Spec: &AutoUpdateImageSpec{
			TargetImgSelector: &TargetImgSelector{},
			RemoteTagSelector: &RemoteTagSelector{
				Sort:  "alphabetically",
				Order: "asc",
			},
		},
	}
}

func MustAtoi(s string) int {
	i, err := strconv.Atoi(s)

	if err != nil {
		log.Fatal(err)
	}

	return i
}

func init() {
	log.SetOutput(os.Stderr)
}

func filter(api *AutoUpdateImage) kio.FilterFunc {
	return func(items []*yaml.RNode) ([]*yaml.RNode, error) {
		for _, item := range items {
			var containers *yaml.RNode
			var err error

			switch item.GetKind() {
			case "Deployment", "StatefulSet", "DaemonSet", "Job":
				containers, err = item.Pipe(yaml.Lookup("spec", "template", "spec", "containers"))
			case "CronJob":
				containers, err = item.Pipe(yaml.Lookup("spec", "jobTemplate", "spec", "template", "spec", "containers"))
			default:
				continue
			}

			if err != nil {
				return nil, err
			}

			err = containers.VisitElements(func(node *yaml.RNode) error {
				image, err := node.GetString("image")

				if err != nil {
					return err
				}

				ref, err := name.ParseReference(image)

				if err != nil {
					return err
				}

				if api.Spec.TargetImgSelector.Pattern != nil && !api.Spec.TargetImgSelector.Pattern.MatchString(ref.String()) {
					return nil
				}

				namespace := item.GetNamespace()

				if namespace == "" {
					namespace = "-"
				}

				itemId := item.GetKind() + "/" + namespace + "/" + item.GetName()

				log.Printf("processing %s: %s", itemId, ref.String())

				images, err := remote.List(ref.Context(),
					remote.WithAuthFromKeychain(
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
					),
				)

				if err != nil {
					return err
				}

				filtered := []string{}

				cmp := api.Spec.RemoteTagSelector.Pattern.SubexpIndex(api.Spec.RemoteTagSelector.Extract)

				for _, image := range images {
					if api.Spec.RemoteTagSelector.Pattern.MatchString(image) {
						filtered = append(filtered, image)
					}
				}

				sort.Slice(filtered, func(i, j int) bool {
					var (
						left  string
						right string
					)

					if cmp == -1 {
						left = filtered[i]
						right = filtered[j]
					} else {
						left = api.Spec.RemoteTagSelector.Pattern.FindStringSubmatch(filtered[i])[cmp]
						right = api.Spec.RemoteTagSelector.Pattern.FindStringSubmatch(filtered[j])[cmp]
					}

					switch api.Spec.RemoteTagSelector.Sort {
					case "numerically":
						left := MustAtoi(left)
						right := MustAtoi(right)

						if api.Spec.RemoteTagSelector.Order == "asc" {
							return left < right
						} else {
							return left > right
						}
					default:
						if api.Spec.RemoteTagSelector.Order == "asc" {
							return left < right
						} else {
							return left > right
						}
					}
				})

				if len(filtered) > 0 {
					current := ref.String()
					next := ref.Context().Tag(filtered[0]).String()
					log.Printf("processed %s: %s -> %s", itemId, current, next)

					if err := node.PipeE(yaml.SetField("image", yaml.NewStringRNode(next))); err != nil {
						return err
					}
				}

				return nil
			})

			if err != nil {
				return nil, err
			}
		}

		return items, nil
	}
}

func main() {
	api := NewAutoUpdateImage()
	p := framework.SimpleProcessor{Config: &api, Filter: kio.FilterFunc(filter(&api))}
	cmd := command.Build(p, command.StandaloneEnabled, false)

	if err := cmd.Execute(); err != nil {
		log.Fatalf("error: %v", err)
		os.Exit(1)
	}
}
