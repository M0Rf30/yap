package abuild

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 - SHA1 required for APK format compatibility with APK-TOOLS.checksum.SHA1
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"time"

	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/M0Rf30/yap/pkg/osutils"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
)

// Apk represents the APK package manager.
//
// It contains the PKGBUILD struct, which contains the metadata and build
// instructions for the package.
type Apk struct {
	// PKGBUILD is a pointer to the pkgbuild.PKGBUILD struct, which contains information about the package being built.
	PKGBUILD *pkgbuild.PKGBUILD
}

// BuildPackage creates an APK package in pure Go without external dependencies.
// It generates the APK package structure and creates a tar.gz archive.
func (a *Apk) BuildPackage(artifactsPath string) error {
	pkgName := a.PKGBUILD.PkgName +
		"-" +
		a.PKGBUILD.PkgVer +
		"-r" +
		a.PKGBUILD.PkgRel +
		"." +
		a.PKGBUILD.ArchComputed +
		".apk"

	pkgFilePath := filepath.Join(artifactsPath, pkgName)

	// Create APK package using pure Go implementation
	err := a.createAPKPackage(pkgFilePath, artifactsPath)
	if err != nil {
		return err
	}

	pkgLogger := osutils.WithComponent(a.PKGBUILD.PkgName)
	pkgLogger.Info("APK package artifact created", osutils.Logger.Args("pkgver", a.PKGBUILD.PkgVer,
		"pkgrel", a.PKGBUILD.PkgRel,
		"artifact", pkgFilePath))

	return nil
}

// PrepareFakeroot sets up the APK package metadata in pure Go.
// It creates the .PKGINFO file and other APK metadata without external tools.
func (a *Apk) PrepareFakeroot(artifactsPath string) error {
	a.PKGBUILD.ArchComputed = APKArchs[a.PKGBUILD.ArchComputed]
	a.PKGBUILD.InstalledSize, _ = osutils.GetDirSize(a.PKGBUILD.PackageDir)
	a.PKGBUILD.BuildDate = time.Now().Unix()
	a.PKGBUILD.PkgDest, _ = filepath.Abs(artifactsPath)
	a.PKGBUILD.YAPVersion = constants.YAPVersion

	// Create .PKGINFO file
	err := a.createPkgInfo()
	if err != nil {
		return err
	}

	// Create install scripts if needed
	if a.PKGBUILD.PreInst != "" || a.PKGBUILD.PostInst != "" || a.PKGBUILD.PreRm != "" || a.PKGBUILD.PostRm != "" {
		err = a.createInstallScript()
		if err != nil {
			return err
		}
	}

	return nil
}

// Install installs the APK package to the specified artifacts path.
//
// It takes a string parameter `artifactsPath` which specifies the path where the artifacts are located.
// It returns an error if there was an error during the installation process.
func (a *Apk) Install(artifactsPath string) error {
	pkgName := a.PKGBUILD.PkgName + "-" +
		a.PKGBUILD.PkgVer +
		"-" +
		"r" + a.PKGBUILD.PkgRel +
		"-" +
		a.PKGBUILD.ArchComputed +
		".apk"

	pkgFilePath := filepath.Join(artifactsPath, a.PKGBUILD.PkgName, a.PKGBUILD.ArchComputed, pkgName)
	installArgs = append(installArgs, pkgFilePath)

	err := osutils.Exec(true, "", "apk", installArgs...)
	if err != nil {
		return err
	}

	return nil
}

// Prepare prepares the Apk by adding dependencies to the PKGBUILD file.
//
// makeDepends is a slice of strings representing the dependencies to be added.
// It returns an error if there is any issue with adding the dependencies.
func (a *Apk) Prepare(makeDepends []string) error {
	return a.PKGBUILD.GetDepends("apk", installArgs, makeDepends)
}

// PrepareEnvironment prepares the build environment for APK packaging.
// It installs requested Go tools if 'golang' is true.
// It returns an error if any step fails.
func (a *Apk) PrepareEnvironment(golang bool) error {
	installArgs = append(installArgs, buildEnvironmentDeps...)

	if golang {
		osutils.CheckGO()

		installArgs = append(installArgs, "go")
	}

	return osutils.Exec(true, "", "apk", installArgs...)
}

// Update updates the APK package manager's package database.
// It returns an error if the update process fails.
func (a *Apk) Update() error {
	return a.PKGBUILD.GetUpdates("apk", "update")
}

// ExportPublicKeyPEM exports a public key to PEM format bytes.
func (a *Apk) ExportPublicKeyPEM(publicKey *rsa.PublicKey) ([]byte, error) {
	// Marshal public key to DER format
	derBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	// Create PEM block
	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derBytes,
	}

	// Encode to PEM format
	var pemBuffer bytes.Buffer

	err = pem.Encode(&pemBuffer, pemBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to encode public key to PEM: %w", err)
	}

	return pemBuffer.Bytes(), nil
}

// CreateSignatureBuilder creates the signature archive builder function.
func (a *Apk) CreateSignatureBuilder(digest []byte, privateKey *rsa.PrivateKey,
	keyName string,
) func(*tar.Writer) error {
	return func(tarWriter *tar.Writer) error {
		// Sign the control digest with RSA PKCS1v15 and SHA1
		signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA1, digest)
		if err != nil {
			return err
		}

		// CRITICAL FIX: Real Alpine APKs use .rsa.pub suffix in signature filename
		// This matches actual Alpine APK signature files like:
		// .SIGN.RSA.alpine-devel@lists.alpinelinux.org-6165ee59.rsa.pub
		signatureFileName := ".SIGN.RSA." + keyName + ".rsa.pub"

		// Create tar header for signature file using USTAR format
		signatureHeader := &tar.Header{
			Name:       signatureFileName,
			Mode:       0o600,
			Size:       int64(len(signature)),
			ModTime:    time.Now(),
			Typeflag:   tar.TypeReg,
			Uid:        0,
			Gid:        0,
			Format:     tar.FormatUSTAR,
			ChangeTime: time.Time{},
			AccessTime: time.Time{},
		}

		err = tarWriter.WriteHeader(signatureHeader)
		if err != nil {
			return err
		}

		_, err = tarWriter.Write(signature)
		if err != nil {
			return err
		}

		// CRITICAL FIX: Do NOT include public key in signature archive
		// Real Alpine APKs only contain the signature file
		// Public key is looked up from /etc/apk/keys/ by filename matching

		return nil
	}
}

// createAPKPackage creates the APK package archive with proper Alpine Linux format.
// APK packages are three separate gzip-compressed tar archives concatenated together:
// signature.tar.gz + control.tar.gz + data.tar.gz.
func (a *Apk) createAPKPackage(pkgFilePath, artifactsPath string) error {
	// Step 1: Calculate datahash of the data files for .PKGINFO
	dataHashStr, err := a.calculateDataHash()
	if err != nil {
		return fmt.Errorf("failed to calculate data hash: %w", err)
	}

	// Step 2: Set the datahash in PKGBUILD so it gets included in .PKGINFO
	a.PKGBUILD.DataHash = dataHashStr

	// Recreate .PKGINFO with datahash
	err = a.createPkgInfo()
	if err != nil {
		return fmt.Errorf("failed to create .PKGINFO with datahash: %w", err)
	}

	// Step 3: Generate temporary RSA key for signing
	privateKey, err := a.generateSigningKey()
	if err != nil {
		return fmt.Errorf("failed to generate signing key: %w", err)
	}

	// Save public key to /etc/apk/keys/ for verification (ignore errors in test environments)
	// Use a unique key name based on package info to avoid conflicts
	keyName := fmt.Sprintf("yap-%s-%s", a.PKGBUILD.PkgName, a.PKGBUILD.PkgVer)

	err = a.savePublicKey(privateKey, keyName)
	if err != nil {
		// Log warning but continue - this is expected in test environments without root permissions
		osutils.Logger.Warn("Failed to save public key (continuing without key installation): " + err.Error())
	}

	// Also save the public key file alongside the APK for distribution
	err = a.savePublicKeyToArtifacts(privateKey, keyName, artifactsPath)
	if err != nil {
		osutils.Logger.Warn("Failed to save public key to artifacts: " + err.Error())
	}

	// Step 4: Create the final APK file
	// G304: Package file paths are controlled within package build process
	outFile, err := os.Create(pkgFilePath)
	if err != nil {
		return fmt.Errorf("failed to create APK file: %w", err)
	}
	defer outFile.Close()

	// Step 5: Create data tar.gz archive first (needed for control)
	var dataBuf bytes.Buffer

	dataDigest, err := a.createDataArchive(&dataBuf)
	if err != nil {
		return fmt.Errorf("failed to create data archive: %w", err)
	}

	// Step 6: Create control tar.gz archive
	var controlBuf bytes.Buffer

	controlDigest, err := a.createControlArchive(&controlBuf, int64(dataBuf.Len()), dataDigest)
	if err != nil {
		return fmt.Errorf("failed to create control archive: %w", err)
	}

	// Step 7: Create signature tar.gz archive
	var signatureBuf bytes.Buffer

	err = a.createSignatureArchive(&signatureBuf, privateKey, controlDigest, keyName)
	if err != nil {
		return fmt.Errorf("failed to create signature archive: %w", err)
	}

	// Step 8: Concatenate the three archives: signature + control + data
	err = a.combineArchives(outFile, &signatureBuf, &controlBuf, &dataBuf)
	if err != nil {
		return fmt.Errorf("failed to combine archives: %w", err)
	}

	return nil
}

// calculateDataHash calculates SHA256 hash of all data files for the datahash field.
func (a *Apk) calculateDataHash() (string, error) {
	hasher := sha256.New()

	err := filepath.WalkDir(a.PKGBUILD.PackageDir, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == a.PKGBUILD.PackageDir {
			return nil
		}

		// Skip control files - only hash data files
		fileName := filepath.Base(path)
		if fileName == ".PKGINFO" || fileName[0] == '.' {
			return nil
		}

		relPath, err := filepath.Rel(a.PKGBUILD.PackageDir, path)
		if err != nil {
			return err
		}

		fileInfo, err := dirEntry.Info()
		if err != nil {
			return err
		}

		// Write file path and metadata to hasher
		hasher.Write([]byte(relPath))
		hasher.Write([]byte{byte(fileInfo.Mode())})

		// Hash file content if it's a regular file
		if fileInfo.Mode().IsRegular() {
			// G304: File paths are controlled within package build process
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(hasher, file)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// generateSigningKey generates a temporary RSA key pair for signing.
func (a *Apk) generateSigningKey() (*rsa.PrivateKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

// savePublicKey saves the public key to /etc/apk/keys/ for signature verification.
func (a *Apk) savePublicKey(privateKey *rsa.PrivateKey, keyName string) error {
	publicKey := &privateKey.PublicKey

	// For APK compatibility, we need to save the key in PEM format (like official Alpine keys)
	derBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}

	// Create PEM block (Alpine uses PEM format for public keys)
	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derBytes,
	}

	// Ensure /etc/apk/keys directory exists
	err = os.MkdirAll("/etc/apk/keys", 0o750)
	if err != nil {
		return fmt.Errorf("failed to create /etc/apk/keys directory: %w", err)
	}

	// Write PEM-encoded public key file
	apkKeysDir := "/etc/apk/keys"
	keyPath := filepath.Join(apkKeysDir, keyName+".rsa.pub")

	// G304: Key file paths are controlled within package build process
	keyFile, err := os.Create(keyPath)
	if err != nil {
		return fmt.Errorf("failed to create public key file: %w", err)
	}
	defer keyFile.Close()

	err = pem.Encode(keyFile, pemBlock)
	if err != nil {
		return fmt.Errorf("failed to encode public key: %w", err)
	}

	return nil
}

// savePublicKeyToArtifacts saves the public key to the artifacts directory for distribution.
func (a *Apk) savePublicKeyToArtifacts(privateKey *rsa.PrivateKey, keyName, artifactsPath string) error {
	publicKey := &privateKey.PublicKey

	// For APK compatibility, save the key in PEM format (like official Alpine keys)
	derBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}

	// Create PEM block
	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derBytes,
	}

	// Write PEM-encoded public key file to artifacts directory
	keyPath := filepath.Join(artifactsPath, keyName+".rsa.pub")

	// G304: Key file paths are controlled within package build process
	keyFile, err := os.Create(keyPath)
	if err != nil {
		return fmt.Errorf("failed to create public key file in artifacts: %w", err)
	}
	defer keyFile.Close()

	err = pem.Encode(keyFile, pemBlock)
	if err != nil {
		return fmt.Errorf("failed to encode public key: %w", err)
	}

	osutils.Logger.Info("Public key saved to artifacts", osutils.Logger.Args("path", keyPath))

	return nil
}

// createPkgInfo creates the .PKGINFO file for the APK package.
func (a *Apk) createPkgInfo() error {
	tmpl := a.PKGBUILD.RenderSpec(pkgInfoTemplate)
	pkgInfoPath := filepath.Join(a.PKGBUILD.PackageDir, ".PKGINFO")

	return a.PKGBUILD.CreateSpec(pkgInfoPath, tmpl)
}

// createInstallScript creates install scripts for the APK package.
func (a *Apk) createInstallScript() error {
	if a.PKGBUILD.PreInst == "" && a.PKGBUILD.PostInst == "" && a.PKGBUILD.PreRm == "" && a.PKGBUILD.PostRm == "" {
		return nil
	}

	// Create pre-install script
	if a.PKGBUILD.PreInst != "" {
		preInstPath := filepath.Join(a.PKGBUILD.PackageDir, ".pre-install")

		err := os.WriteFile(preInstPath, []byte("#!/bin/sh\n"+a.PKGBUILD.PreInst), 0o600)
		if err != nil {
			return err
		}
	}

	// Create post-install script
	if a.PKGBUILD.PostInst != "" {
		postInstPath := filepath.Join(a.PKGBUILD.PackageDir, ".post-install")

		err := os.WriteFile(postInstPath, []byte("#!/bin/sh\n"+a.PKGBUILD.PostInst), 0o600)
		if err != nil {
			return err
		}
	}

	// Create pre-remove script
	if a.PKGBUILD.PreRm != "" {
		preRmPath := filepath.Join(a.PKGBUILD.PackageDir, ".pre-deinstall")

		err := os.WriteFile(preRmPath, []byte("#!/bin/sh\n"+a.PKGBUILD.PreRm), 0o600)
		if err != nil {
			return err
		}
	}

	// Create post-remove script
	if a.PKGBUILD.PostRm != "" {
		postRmPath := filepath.Join(a.PKGBUILD.PackageDir, ".post-deinstall")

		err := os.WriteFile(postRmPath, []byte("#!/bin/sh\n"+a.PKGBUILD.PostRm), 0o600)
		if err != nil {
			return err
		}
	}

	return nil
}

// writerCounter tracks bytes written for tar alignment.
type writerCounter struct {
	io.Writer

	count  uint64
	writer io.Writer
}

func newWriterCounter(w io.Writer) *writerCounter {
	return &writerCounter{
		writer: w,
	}
}

func (counter *writerCounter) Write(buf []byte) (int, error) {
	n, err := counter.writer.Write(buf)
	if n >= 0 {
		atomic.AddUint64(&counter.count, uint64(n))
	}

	return n, err
}

func (counter *writerCounter) Count() uint64 {
	return atomic.LoadUint64(&counter.count)
}

// tarKind represents different tar archive types.
type tarKind int

const (
	tarFull tarKind = iota
	tarCut
)

// writeTgz creates a gzip-compressed tar archive with proper alignment.
func (a *Apk) writeTgz(w io.Writer, kind tarKind, builder func(tarWriter *tar.Writer) error,
	digest hash.Hash,
) ([]byte, error) {
	mw := io.MultiWriter(digest, w)
	gzipWriter := gzip.NewWriter(mw)
	counterWriter := newWriterCounter(gzipWriter)
	bufWriter := bufio.NewWriterSize(counterWriter, 4096)
	tarWriter := tar.NewWriter(bufWriter)

	err := builder(tarWriter)
	if err != nil {
		return nil, err
	}

	// Handle the cut vs full tars
	err = bufWriter.Flush()
	if err != nil {
		return nil, err
	}

	err = tarWriter.Close()
	if err != nil {
		return nil, err
	}

	if kind == tarFull {
		err = bufWriter.Flush()
		if err != nil {
			return nil, err
		}
	}

	size := counterWriter.Count()
	alignedSize := (size + 511) & ^uint64(511)

	increase := alignedSize - size
	if increase > 0 {
		b := make([]byte, increase)

		_, err = counterWriter.Write(b)
		if err != nil {
			return nil, err
		}
	}

	err = gzipWriter.Close()
	if err != nil {
		return nil, err
	}

	return digest.Sum(nil), nil
}

// createDataArchive creates the data tar.gz archive.
func (a *Apk) createDataArchive(dataTgz io.Writer) ([]byte, error) {
	builderData := a.createBuilderData()

	dataDigest, err := a.writeTgz(dataTgz, tarFull, builderData, sha256.New())
	if err != nil {
		return nil, err
	}

	return dataDigest, nil
}

// createControlArchive creates the control tar.gz archive.
func (a *Apk) createControlArchive(controlTgz io.Writer, size int64, dataDigest []byte) ([]byte, error) {
	builderControl := a.createBuilderControl(size, dataDigest)

	// G401: SHA1 required for APK format compatibility
	controlDigest, err := a.writeTgz(controlTgz, tarCut, builderControl, sha1.New())
	if err != nil {
		return nil, err
	}

	return controlDigest, nil
}

// createSignatureArchive creates the signature tar.gz archive.
func (a *Apk) createSignatureArchive(signatureTgz io.Writer, privateKey *rsa.PrivateKey,
	controlSHA1Digest []byte, keyName string,
) error {
	signatureBuilder := a.CreateSignatureBuilder(controlSHA1Digest, privateKey, keyName)

	// G401: SHA1 required for APK format compatibility
	_, err := a.writeTgz(signatureTgz, tarCut, signatureBuilder, sha1.New())
	if err != nil {
		return fmt.Errorf("signing failure: %w", err)
	}

	return nil
}

// combineArchives concatenates the three tar.gz archives.
func (a *Apk) combineArchives(target io.Writer, readers ...io.Reader) error {
	for _, tgz := range readers {
		_, err := io.Copy(target, tgz)
		if err != nil {
			return err
		}
	}

	return nil
}

// createBuilderData creates the data archive builder function.
func (a *Apk) createBuilderData() func(tarWriter *tar.Writer) error {
	return func(tarWriter *tar.Writer) error {
		return a.addDataFilesToTarWriter(tarWriter)
	}
}

// createBuilderControl creates the control archive builder function.
func (a *Apk) createBuilderControl(_ int64, _ []byte) func(tarWriter *tar.Writer) error {
	return func(tarWriter *tar.Writer) error {
		// Add .PKGINFO file
		pkgInfoPath := filepath.Join(a.PKGBUILD.PackageDir, ".PKGINFO")

		err := a.addFileToTarWriter(tarWriter, pkgInfoPath, ".PKGINFO")
		if err != nil {
			return err
		}

		// Add install scripts if they exist
		scripts := map[string]string{
			".pre-install":    ".pre-install",
			".post-install":   ".post-install",
			".pre-deinstall":  ".pre-deinstall",
			".post-deinstall": ".post-deinstall",
		}

		for scriptFile, tarName := range scripts {
			scriptPath := filepath.Join(a.PKGBUILD.PackageDir, scriptFile)

			_, statErr := os.Stat(scriptPath)
			if statErr == nil {
				err = a.addFileToTarWriter(tarWriter, scriptPath, tarName)
				if err != nil {
					return err
				}
			}
		}

		return nil
	}
}

// addDataFilesToTarWriter adds all data files to the tar writer with SHA1 checksums.
func (a *Apk) addDataFilesToTarWriter(tarWriter *tar.Writer) error {
	return filepath.WalkDir(a.PKGBUILD.PackageDir, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == a.PKGBUILD.PackageDir {
			return nil
		}

		// Skip control files - they were already added
		if a.isControlFile(filepath.Base(path)) {
			return nil
		}

		relPath, err := filepath.Rel(a.PKGBUILD.PackageDir, path)
		if err != nil {
			return err
		}

		fileInfo, err := dirEntry.Info()
		if err != nil {
			return err
		}

		return a.addFileToTarWithChecksum(tarWriter, path, relPath, fileInfo)
	})
}

// isControlFile checks if a file is a control file that should be skipped.
func (a *Apk) isControlFile(fileName string) bool {
	if fileName == ".PKGINFO" {
		return true
	}

	if fileName[0] != '.' {
		return false
	}

	controlScripts := []string{
		".pre-install",
		".post-install",
		".pre-deinstall",
		".post-deinstall",
	}

	return slices.Contains(controlScripts, fileName)
}

// addFileToTarWithChecksum adds a single file to the tar archive with proper headers and checksums.
func (a *Apk) addFileToTarWithChecksum(tarWriter *tar.Writer, path, relPath string, fileInfo fs.FileInfo) error {
	// Create tar header
	header, err := tar.FileInfoHeader(fileInfo, "")
	if err != nil {
		return err
	}

	header.Name = relPath
	header.Uid = 0
	header.Gid = 0
	header.Uname = "root"
	header.Gname = "root"

	// Handle symlinks
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return a.handleSymlink(tarWriter, path, header)
	}

	// Add SHA1 checksum for regular files
	if fileInfo.Mode().IsRegular() {
		return a.handleRegularFile(tarWriter, path, header)
	}

	// For directories and special files, use USTAR format without PAX records
	header.Format = tar.FormatUSTAR
	header.ChangeTime = time.Time{}
	header.AccessTime = time.Time{}

	return tarWriter.WriteHeader(header)
}

// handleSymlink processes a symbolic link and adds it to the tar archive.
func (a *Apk) handleSymlink(tarWriter *tar.Writer, path string, header *tar.Header) error {
	linkTarget, err := os.Readlink(path)
	if err != nil {
		return err
	}

	header.Linkname = linkTarget
	// Use PAX format for symlinks to include checksum
	header.Format = tar.FormatPAX
	if header.PAXRecords == nil {
		header.PAXRecords = make(map[string]string)
	}

	// Calculate SHA1 checksum for empty content (symlinks have no content)
	sha1Hash := sha1.Sum([]byte{}) // #nosec G401 - SHA1 required for APK-TOOLS.checksum.SHA1 format
	sha1Hex := hex.EncodeToString(sha1Hash[:])
	header.PAXRecords["APK-TOOLS.checksum.SHA1"] = sha1Hex

	return tarWriter.WriteHeader(header)
}

// handleRegularFile processes a regular file, calculates its checksum, and adds it to the tar archive.
func (a *Apk) handleRegularFile(tarWriter *tar.Writer, path string, header *tar.Header) error {
	// Read file content to calculate SHA1 checksum
	// G304: File paths are controlled within package build process
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	fileContent, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	// Calculate SHA1 checksum - required for APK format compatibility
	sha1Hash := sha1.Sum(fileContent) // #nosec G401 - SHA1 required for APK-TOOLS.checksum.SHA1 format
	sha1Hex := hex.EncodeToString(sha1Hash[:])

	// Set PAX format for files with checksums (data files)
	header.Format = tar.FormatPAX
	if header.PAXRecords == nil {
		header.PAXRecords = make(map[string]string)
	}

	header.PAXRecords["APK-TOOLS.checksum.SHA1"] = sha1Hex

	err = tarWriter.WriteHeader(header)
	if err != nil {
		return err
	}

	// Write file content
	_, err = tarWriter.Write(fileContent)
	if err != nil {
		return err
	}

	return nil
}

// addFileToTarWriter adds a file to a tar writer with SHA1 checksum in extended headers.
func (a *Apk) addFileToTarWriter(tarWriter *tar.Writer, filePath, tarPath string) error {
	// G304: File paths are controlled within package build process
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	// Read file content to calculate SHA1 checksum
	fileContent, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	// For control files (.PKGINFO, scripts), use USTAR format without PAX records
	if a.isControlFile(filepath.Base(tarPath)) {
		header := &tar.Header{
			Name:       tarPath,
			Mode:       int64(fileInfo.Mode()),
			Size:       fileInfo.Size(),
			ModTime:    fileInfo.ModTime(),
			Typeflag:   tar.TypeReg,
			Uid:        0,
			Gid:        0,
			Uname:      "root",
			Gname:      "root",
			Format:     tar.FormatUSTAR,
			ChangeTime: time.Time{},
			AccessTime: time.Time{},
		}

		err = tarWriter.WriteHeader(header)
		if err != nil {
			return err
		}

		// Write file content
		_, err = tarWriter.Write(fileContent)

		return err
	}

	// For data files, use PAX format with SHA1 checksum
	sha1Hash := sha1.Sum(fileContent) // #nosec G401 - SHA1 required for APK-TOOLS.checksum.SHA1 format
	sha1Hex := hex.EncodeToString(sha1Hash[:])

	// Create header with PAX extended header for SHA1 checksum
	header := &tar.Header{
		Name:     tarPath,
		Mode:     int64(fileInfo.Mode()),
		Size:     fileInfo.Size(),
		ModTime:  fileInfo.ModTime(),
		Typeflag: tar.TypeReg,
		Uid:      0,
		Gid:      0,
		Uname:    "root",
		Gname:    "root",
		Format:   tar.FormatPAX,
		// PAX extended attributes for APK-TOOLS checksum
		PAXRecords: map[string]string{
			"APK-TOOLS.checksum.SHA1": sha1Hex,
		},
	}

	err = tarWriter.WriteHeader(header)
	if err != nil {
		return err
	}

	// Write file content
	_, err = tarWriter.Write(fileContent)

	return err
}
