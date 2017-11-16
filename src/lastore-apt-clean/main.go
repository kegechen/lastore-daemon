package main

import (
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"os/exec"
	"os"
	"time"
	"syscall"
	"bytes"
	"bufio"
	"errors"
)

const maxElapsed = time.Hour * 24 * 6 // 6 days

var (
	archivesDir string
	binDpkg string
	binDpkgQuery string
	binAptCache string
	binAptConfig string
)

func mustGetBin(name string) string {
	file, err := exec.LookPath(name)
	if err != nil {
		log.Fatal(err)
	}
	return file
}

func main() {
	log.SetFlags(log.Lshortfile)
	binDpkg = mustGetBin("dpkg")
	binDpkgQuery = mustGetBin("dpkg-query")
	binAptCache = mustGetBin("apt-cache")
	binAptConfig = mustGetBin("apt-config")

	os.Setenv("LC_ALL", "C")

	var err error
	archivesDir, err = getArchivesDir()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("archives dir:", archivesDir)

	fileInfoList, err := ioutil.ReadDir(archivesDir)
	if err != nil {
		log.Fatal(err)
	}

	for _, fileInfo := range fileInfoList {
		if fileInfo.IsDir() {
			continue
		}

		if filepath.Ext(fileInfo.Name()) != ".deb" {
			continue
		}

		log.Println("> ", fileInfo.Name())
		del, err := shouldDelete(fileInfo)
		if err != nil {
			log.Println("shouldDelete error:", err)
			continue
		}
		if del {
			deleteDeb(fileInfo.Name())
		}

	}
}

/*
$ apt-config --format '%f=%v%n' dump  Dir
Dir=/
Dir::Cache=var/cache/apt
Dir::Cache::archives=archives/
Dir::Cache::srcpkgcache=srcpkgcache.bin
Dir::Cache::pkgcache=pkgcache.bin
*/
func getArchivesDir() (string, error) {
	output, err := exec.Command(binAptConfig, "--format", "%f=%v%n", "dump", "Dir").Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(output), "\n")
	tempMap := make(map[string]string)
	for _,line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			switch parts[0] {
			case "Dir", "Dir::Cache", "Dir::Cache::archives":
				tempMap[parts[0]] = parts[1]
			}
		}
	}
	dir := tempMap["Dir"]
	if dir == "" {
		return "", errors.New("apt-config Dir is empty")
	}

	dirCache := tempMap["Dir::Cache"]
	if dirCache == "" {
		return "", errors.New("apt-config Dir::Cache is empty")
	}
	dirCacheArchives := tempMap["Dir::Cache::archives"]
	if dirCacheArchives == "" {
		return "", errors.New("apt-config Dir::Cache::Archives is empty")
	}

	return filepath.Join(dir, dirCache, dirCacheArchives), nil
}

func shouldDelete(fileInfo os.FileInfo) (bool, error) {
	debInfo := getDebFileNameInfo(fileInfo.Name())
	log.Printf("%#v\n", debInfo)

	installedVersion, _ := getInstalledVersion(debInfo)

	if installedVersion != "" {
		log.Println("installed version:", installedVersion)

		if compareVersions(debInfo.version, "gt", installedVersion) {
			log.Println("deb version great then installed version")
			candidateVersion, err := getCandidateVersion(debInfo)
			if err != nil {
				return false, err
			}

			log.Println("candidate version:", candidateVersion)
			if  candidateVersion != debInfo.version {
				log.Println("not the candiate version")
				return true, nil
			}
			return false, nil
		} else {
			return true, nil
		}

	} else {
		log.Println("package not installed")
		// removed or newly added
		debChangeTime := getChangeTime(fileInfo)
		elapsed := time.Since(debChangeTime)
		return elapsed > maxElapsed, nil
	}
}

type DebFileNameInfo struct {
	name string
	version string
	arch string
}

func getDebFileNameInfo(basename string) *DebFileNameInfo {
	basename = strings.TrimSuffix(basename, ".deb")
	parts := strings.Split(basename, "_")

	if len(parts) != 3 {
		return nil
	}

	var info DebFileNameInfo
	info.name = parts[0]
	if info.name == "" {
		return nil
	}

	info.version = parts[1]
	if info.version == "" {
		return nil
	}

	info.arch = parts[2]
	if info.arch == "" {
		return nil
	}

	return &info
}

func getInstalledVersion(info *DebFileNameInfo) (string, error)  {
	pkg := info.name + ":" + info.arch
	output, err := exec.Command(binDpkgQuery, "-f", "${Version}", "-W", pkg).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func getCandidateVersion(info *DebFileNameInfo) (string, error) {
	pkg := info.name + ":" + info.arch
	output, err := exec.Command(binAptCache, "policy", "--", pkg).Output()
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		const candidate = "Candidate:"
		if strings.HasPrefix(line, candidate) {
			return strings.TrimSpace(line[len(candidate):]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("not found candidate")
}

func compareVersions(ver1, op, ver2 string) bool {
	err := exec.Command(binDpkg, "--compare-versions", ver1, op, ver2).Run()
	return err == nil
}

// getChangeTime get time when file status was last changed.
func getChangeTime(fileInfo os.FileInfo) time.Time {
	stat := fileInfo.Sys().(*syscall.Stat_t)
	return time.Unix(int64(stat.Ctim.Sec), int64(stat.Ctim.Nsec))
}

func deleteDeb(name string) {
	log.Println("delete deb", name)
	err := os.Remove(filepath.Join(archivesDir, name))
	if err != nil {
		log.Printf("deleteDeb error: %v\n", err)
	}
}