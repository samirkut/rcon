package container

import (
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"rcon/utils"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/uuid"
)

func FetchContainer(imageRef, cacheDir string, skipCache bool) error {
	imageFolderLink := getImageDir(cacheDir, imageRef)
	if skipCache && utils.PathExists(imageFolderLink) {
		return nil
	}

	// download image manifest
	img, err := crane.Pull(imageRef)
	if err != nil {
		return err
	}

	imgHash, err := img.ConfigName()
	if err != nil {
		return err
	}

	imgId := imgHash.String()

	// download and export if not in cache
	exportDir := filepath.Join(cacheDir, imgId)

	os.MkdirAll(exportDir, 0755)
	tarFile := filepath.Join(exportDir, "fs.tar")
	if !utils.PathExists(tarFile) {
		f, err := os.Create(tarFile)
		if err != nil {
			return err
		}

		err = crane.Export(img, f)
		if err != nil {
			f.Close()
			return err
		}

		f.Close()
	}

	// extract config
	configFilePath := filepath.Join(exportDir, "config.json")
	if !utils.PathExists(configFilePath) {
		data, err := img.RawConfigFile()
		if err != nil {
			return err
		}

		err = os.WriteFile(configFilePath, data, fs.ModePerm)
		if err != nil {
			return err
		}
	}

	if utils.PathExists(imageFolderLink) {
		// extract old symlink target. we should remove it and delete the old files
		oldPath, err := filepath.EvalSymlinks(imageFolderLink)
		if err != nil {
			return err
		}

		// if the symlink is the same as exportDir we can skipt
		if oldPath == exportDir {
			return nil
		}

		err = os.Remove(imageFolderLink)
		if err != nil {
			return err
		}

		err = os.RemoveAll(oldPath)
		if err != nil {
			return err
		}
	}

	// symlink imageRef -> imgId
	return os.Symlink(exportDir, imageFolderLink)
}

func PrepContainer(imageRef, cacheDir, runDir string) (string, *v1.Config, error) {
	imgDir := getImageDir(cacheDir, imageRef)
	tarFile := filepath.Join(imgDir, "fs.tar")

	// extract filesystem
	instanceId := uuid.NewString()
	rootFS := filepath.Join(runDir, instanceId)
	os.MkdirAll(rootFS, 0755)
	err := utils.Untar(tarFile, rootFS)
	if err != nil {
		return "", nil, err
	}

	// load config
	cfgFilePath := filepath.Join(imgDir, "config.json")
	data, err := os.ReadFile(cfgFilePath)
	if err != nil {
		return "", nil, err
	}

	cfgFile := v1.ConfigFile{}
	err = json.Unmarshal(data, &cfgFile)
	if err != nil {
		return "", nil, err
	}

	return rootFS, &cfgFile.Config, nil
}

func getImageDir(cacheDir, imageRef string) string {
	imageRefHash := base64.StdEncoding.EncodeToString([]byte(imageRef))
	return filepath.Join(cacheDir, imageRefHash)
}