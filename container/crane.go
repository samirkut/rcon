package container

import (
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"rcon/utils"
)

func FetchContainer(imageRef, cacheDir, authFile string, skipCache bool) error {
	imageFolderLink := getImageDir(cacheDir, imageRef)
	if !skipCache && utils.PathExists(imageFolderLink) {
		logger.Tracef("Skip fetch of container %s", imageRef)
		return nil
	}

	logger.Infof("Fetching container %s", imageRef)

	// load auth file if provided
	kc := authn.NewMultiKeychain(
		authn.DefaultKeychain,
		authn.NewKeychainFromHelper(&AuthHelper{AuthFile: authFile}),
	)

	opts := []crane.Option{crane.WithAuthFromKeychain(kc)}

	// download image manifest
	img, err := crane.Pull(imageRef, opts...)
	if err != nil {
		return err
	}

	imgHash, err := img.ConfigName()
	if err != nil {
		return err
	}

	imgId := imgHash.String()
	logger.Infof("Fetcched image with hash: %s", imgId)

	// download and export if not in cache
	exportDir := filepath.Join(cacheDir, imgId)

	os.MkdirAll(exportDir, 0755)
	tarFile := filepath.Join(exportDir, "fs.tar")
	if !utils.PathExists(tarFile) {
		logger.Info("Exporting filesystem as tar")
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
		logger.Info("Exporting config file")
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

		// if the symlink is the same as exportDir we can skip
		if oldPath == exportDir {
			logger.Info("Skipping as it already exists")
			return nil
		}

		err = os.Remove(imageFolderLink)
		if err != nil {
			return err
		}

		logger.Infof("Removing old image at %s", oldPath)
		err = os.RemoveAll(oldPath)
		if err != nil {
			return err
		}
	}

	// symlink imageRef -> imgId
	// convert exportDir to relative path (do we care?)
	relExportDir, err := filepath.Rel(filepath.Dir(imageFolderLink), exportDir)
	if err == nil {
		return os.Symlink(relExportDir, imageFolderLink)
	} else {
		logger.Warnf("failed to convert %s to relative path with basepath %s", exportDir, imageFolderLink)
		return os.Symlink(exportDir, imageFolderLink)
	}
}

func PrepContainer(imageRef, cacheDir, rootFS string) (string, *v1.Config, error) {
	logger.Tracef("Running prep container for %s", imageRef)

	imgDir := getImageDir(cacheDir, imageRef)
	tarFile := filepath.Join(imgDir, "fs.tar")

	// extract filesystem
	os.MkdirAll(rootFS, 0755)

	// get tar file size
	tarSize, err := utils.FileSize(tarFile)
	if err != nil {
		return "", nil, err
	}

	// create tmpfs which is roughly 2 x tarsize
	err = MountTmpfs(rootFS, tarSize*2, true)
	if err != nil {
		return "", nil, err
	}

	err = utils.Untar(tarFile, rootFS)
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
