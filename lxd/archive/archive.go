package archive

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/canonical/lxd/lxd/apparmor"
	"github.com/canonical/lxd/lxd/state"
	"github.com/canonical/lxd/lxd/subprocess"
	"github.com/canonical/lxd/shared"
	"github.com/canonical/lxd/shared/ioprogress"
	"github.com/canonical/lxd/shared/logger"
)

type nullWriteCloser struct {
	*bytes.Buffer
}

// Close closes the null IO writer.
func (nwc *nullWriteCloser) Close() error {
	return nil
}

// ExtractWithFds runs extractor process under specifc AppArmor profile.
// The allowedCmds argument specify commands which are allowed to run by apparmor.
// The cmd argument is automatically added to allowedCmds slice.
func ExtractWithFds(s *state.State, cmd string, args []string, allowedCmds []string, stdin io.ReadCloser, output *os.File) error {
	outputPath := output.Name()

	allowedCmds = append(allowedCmds, cmd)
	allowedCmdPaths := []string{}
	for _, c := range allowedCmds {
		cmdPath, err := exec.LookPath(c)
		if err != nil {
			return fmt.Errorf("Failed to start extract: Failed to find executable: %w", err)
		}

		allowedCmdPaths = append(allowedCmdPaths, cmdPath)
	}

	err := apparmor.ArchiveLoad(s, outputPath, allowedCmdPaths)
	if err != nil {
		return fmt.Errorf("Failed to start extract: Failed to load profile: %w", err)
	}

	defer func() { _ = apparmor.ArchiveDelete(s.OS, outputPath) }()
	defer func() { _ = apparmor.ArchiveUnload(s.OS, outputPath) }()

	var buffer bytes.Buffer
	p := subprocess.NewProcessWithFds(cmd, args, stdin, output, &nullWriteCloser{&buffer})
	p.SetApparmor(apparmor.ArchiveProfileName(outputPath))

	err = p.Start(context.TODO())
	if err != nil {
		return fmt.Errorf("Failed to start extract: Failed running: tar: %w", err)
	}

	_, err = p.Wait(context.Background())
	if err != nil {
		return shared.NewRunError(cmd, args, err, nil, &buffer)
	}

	return nil
}

// CompressedTarReader returns a tar reader from the supplied (optionally compressed) tarball stream.
// The unpacker arguments are those returned by DetectCompressionFile().
// The returned cancelFunc should be called when finished with reader to clean up any resources used.
// This can be done before reading to the end of the tarball if desired.
func CompressedTarReader(s *state.State, ctx context.Context, r io.ReadSeeker, unpacker []string, outputPath string) (*tar.Reader, context.CancelFunc, error) {
	ctx, cancelFunc := context.WithCancel(ctx)

	_, err := r.Seek(0, io.SeekStart)
	if err != nil {
		return nil, cancelFunc, err
	}

	var tr *tar.Reader

	if len(unpacker) > 0 {
		cmdPath, err := exec.LookPath(unpacker[0])
		if err != nil {
			return nil, cancelFunc, fmt.Errorf("Failed to start unpack: Failed to find executable: %w", err)
		}

		err = apparmor.ArchiveLoad(s, outputPath, []string{cmdPath})
		if err != nil {
			return nil, cancelFunc, fmt.Errorf("Failed to start unpack: Failed to load profile: %w", err)
		}

		pipeReader, pipeWriter := io.Pipe()
		p := subprocess.NewProcessWithFds(unpacker[0], unpacker[1:], io.NopCloser(r), pipeWriter, nil)
		p.SetApparmor(apparmor.ArchiveProfileName(outputPath))
		err = p.Start(ctx)
		if err != nil {
			return nil, cancelFunc, fmt.Errorf("Failed to start unpack: Failed running: %s: %w", unpacker[0], err)
		}

		ctxCancelFunc := cancelFunc

		// Now that unpacker process has started, wrap context cancel function with one that waits for
		// the unpacker process to complete.
		cancelFunc = func() {
			ctxCancelFunc()
			_ = pipeWriter.Close()
			_, _ = p.Wait(ctx)
			_ = apparmor.ArchiveUnload(s.OS, outputPath)
			_ = apparmor.ArchiveDelete(s.OS, outputPath)
		}

		tr = tar.NewReader(pipeReader)
	} else {
		tr = tar.NewReader(r)
	}

	return tr, cancelFunc, nil
}

// Unpack extracts image from archive.
func Unpack(s *state.State, file string, path string, blockBackend bool, tracker *ioprogress.ProgressTracker) error {
	extractArgs, extension, unpacker, err := shared.DetectCompression(file)
	if err != nil {
		return err
	}

	command := ""
	args := []string{}
	var allowedCmds []string
	var reader io.Reader
	if strings.HasPrefix(extension, ".tar") {
		command = "tar"
		if s.OS.RunningInUserNS {
			// We can't create char/block devices so avoid extracting them.
			args = append(args, "--anchored")
			args = append(args, "--wildcards")
			args = append(args, "--exclude=dev/*")
			args = append(args, "--exclude=/dev/*")
			args = append(args, "--exclude=./dev/*")
			args = append(args, "--exclude=rootfs/dev/*")
			args = append(args, "--exclude=/rootfs/dev/*")
			args = append(args, "--exclude=./rootfs/dev/*")
		}

		args = append(args, "--restrict", "--force-local")
		args = append(args, "-C", path, "--numeric-owner", "--xattrs-include=*")
		args = append(args, extractArgs...)
		args = append(args, "-")

		f, err := os.Open(file)
		if err != nil {
			return err
		}

		defer func() { _ = f.Close() }()

		reader = f

		// Attach the ProgressTracker if supplied.
		if tracker != nil {
			fsinfo, err := f.Stat()
			if err != nil {
				return err
			}

			tracker.Length = fsinfo.Size()
			reader = &ioprogress.ProgressReader{
				ReadCloser: f,
				Tracker:    tracker,
			}
		}

		// Allow supplementary commands for the unpacker to use.
		if len(unpacker) > 0 {
			allowedCmds = append(allowedCmds, unpacker[0])
		}
	} else if strings.HasPrefix(extension, ".squashfs") {
		// unsquashfs does not support reading from stdin,
		// so ProgressTracker is not possible.
		command = "unsquashfs"
		args = append(args, "-f", "-d", path, "-n")

		// Limit unsquashfs chunk size to 10% of memory and up to 256MiB (default)
		// When running on a low memory system, also disable multi-processing
		mem, err := shared.DeviceTotalMemory()
		mem = mem / 1024 / 1024 / 10
		if err == nil && mem < 256 {
			memString := strconv.FormatInt(mem, 10)
			args = append(args, "-da", memString, "-fr", memString, "-p", "1")
		}

		args = append(args, file)
	} else {
		return fmt.Errorf("Unsupported image format: %s", extension)
	}

	outputDir, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("Error opening directory: %w", err)
	}

	defer func() { _ = outputDir.Close() }()

	var readCloser io.ReadCloser
	if reader != nil {
		readCloser = io.NopCloser(reader)
	}

	err = ExtractWithFds(s, command, args, allowedCmds, readCloser, outputDir)
	if err != nil {
		// We can't create char/block devices in unpriv containers so ignore related errors.
		if s.OS.RunningInUserNS && command == "unsquashfs" {
			runError, ok := err.(shared.RunError)
			if !ok {
				return err
			}

			stdErr := runError.StdErr().String()
			if stdErr == "" {
				return err
			}

			// Confirm that all errors are related to character or block devices.
			found := false
			for line := range strings.SplitSeq(stdErr, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				if !strings.Contains(line, "failed to create block device") {
					continue
				}

				if !strings.Contains(line, "failed to create character device") {
					continue
				}

				// We found an actual error.
				found = true
			}

			if !found {
				// All good, assume everything unpacked.
				return nil
			}
		}

		// Check if we ran out of space
		fs := unix.Statfs_t{}

		err1 := unix.Statfs(path, &fs)
		if err1 != nil {
			return err1
		}

		// Check if we're running out of space
		if int64(fs.Bfree) < 10 {
			if blockBackend {
				return errors.New("Unable to unpack image, run out of disk space (consider increasing your pool's volume.size)")
			}

			return errors.New("Unable to unpack image, run out of disk space")
		}

		logger.Warn("Unpack failed", logger.Ctx{"file": file, "allowedCmds": allowedCmds, "extension": extension, "path": path, "err": err})
		return fmt.Errorf("Unpack failed: %w", err)
	}

	return nil
}
