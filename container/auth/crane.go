package auth

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"regexp"

	"github.com/google/go-containerregistry/pkg/authn"
)

const (
	defaultKey = "default"
)

var (
	domainRegex         = regexp.MustCompile(`^(?:https?://)?([^/:]+)`)
	errCannotCreateAuth = errors.New("cannot create authenticator")
)

type fixedAuthenticator struct {
	cfg *authn.AuthConfig
}

func (a *fixedAuthenticator) Authorization() (*authn.AuthConfig, error) {
	return a.cfg, nil
}

func NewNetcAuthenticator(imageRef string) (authn.Authenticator, error) {
	lines, err := readNetrc()
	if err != nil {
		return nil, err
	}

	domain := getDomainFromImageRef(imageRef)
	if domain == "" {
		return nil, errCannotCreateAuth
	}

	for _, line := range lines {
		if line.machine == domain {
			cfg := &authn.AuthConfig{
				Username: line.login,
				Password: line.password,
			}
			return &fixedAuthenticator{cfg}, nil
		}
	}

	return nil, errCannotCreateAuth
}

func NewFileAuthenticator(authFile, imageRef string) (authn.Authenticator, error) {
	data, err := ioutil.ReadFile(authFile)
	if err != nil {
		return nil, err
	}

	// is this a serialized authConfig
	cfg := &authn.AuthConfig{}
	err = cfg.UnmarshalJSON(data)
	if err == nil {
		return &fixedAuthenticator{cfg}, nil
	}

	// is this a map[string]authConfig where the key refers to the registry/image ref
	mapCfg := make(map[string]*authn.AuthConfig)
	err = json.Unmarshal(data, &mapCfg)
	if err != nil {
		return nil, err
	}

	// check if imageRef exists
	if cfg, ok := mapCfg[imageRef]; ok {
		return &fixedAuthenticator{cfg}, nil
	}

	// see if something begins with the domain
	domain := getDomainFromImageRef(imageRef)
	if domain != "" {
		if cfg, ok := mapCfg[domain]; ok {
			return &fixedAuthenticator{cfg}, nil
		}
	}

	// check for default
	if cfg, ok := mapCfg[defaultKey]; ok {
		return &fixedAuthenticator{cfg}, nil
	}

	return nil, errCannotCreateAuth
}

func getDomainFromImageRef(imageRef string) string {
	for _, m := range domainRegex.FindStringSubmatch(imageRef) {
		return m
	}

	return ""
}
