package operatorkclient

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	helmtar "github.com/Noksa/operator-home/internal/operatortar"
	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// CopyFileToContainerInPod copies a local file into a container.
func (c *Client) CopyFileToContainerInPod(ctx context.Context, pod *corev1.Pod, containerName, srcPath, destPath string, attempts int) error {
	buffer := &bytes.Buffer{}
	srcPath = filepath.Clean(srcPath)
	destPath = filepath.Clean(destPath)
	err := helmtar.Compress(srcPath, destPath, buffer)
	if err != nil {
		return err
	}

	dir := filepath.Dir(destPath)
	req := c.clientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		Param("container", containerName)

	req.VersionedParams(&corev1.PodExecOptions{
		Container: containerName,
		Command: []string{"sh", "-ceu", fmt.Sprintf(`
mkdir -p %v
tar zxf - -C /`, dir)},
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	}, scheme.ParameterCodec)

	return Retry(attempts, time.Second, func() error {
		exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
		if err != nil {
			return err
		}
		b := &strings.Builder{}
		err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:  bytes.NewReader(buffer.Bytes()),
			Stdout: b,
			Stderr: b,
			Tty:    false,
		})
		if err != nil {
			return multierr.Append(err, fmt.Errorf("%s", b.String()))
		}
		return nil
	})
}

// CopyFileFromContainerInPod copies a file from a container to a local path.
func (c *Client) CopyFileFromContainerInPod(ctx context.Context, pod *corev1.Pod, containerName, srcPath, destPath string, attempts int) error {
	srcPath = filepath.Clean(srcPath)
	destPath = filepath.Clean(destPath)

	req := c.clientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		Param("container", containerName)

	req.VersionedParams(&corev1.PodExecOptions{
		Container: containerName,
		Command:   []string{"tar", "zcf", "-", "-C", filepath.Dir(srcPath), filepath.Base(srcPath)},
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)

	return Retry(attempts, time.Second, func() error {
		exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
		if err != nil {
			return err
		}
		buffer := &bytes.Buffer{}
		stderr := &strings.Builder{}
		err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdout: buffer,
			Stderr: stderr,
			Tty:    false,
		})
		if err != nil {
			return multierr.Append(err, fmt.Errorf("%s", stderr.String()))
		}
		return helmtar.Decompress(buffer, destPath)
	})
}
