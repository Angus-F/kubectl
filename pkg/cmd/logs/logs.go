/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/Angus-F/cli-runtime/pkg/genericclioptions"
	"github.com/Angus-F/client-go/rest"
	cmdutil "github.com/Angus-F/kubectl/pkg/cmd/util"
	"github.com/Angus-F/kubectl/pkg/polymorphichelpers"
	"github.com/Angus-F/kubectl/pkg/scheme"
	"github.com/Angus-F/kubectl/pkg/util"
	"github.com/Angus-F/kubectl/pkg/util/i18n"
	"github.com/Angus-F/kubectl/pkg/util/templates"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"github.com/Angus-F/kubectl/pkg/configs"
)

const (
	logsUsageStr = "logs [-f] [-p] (POD | TYPE/NAME) [-c CONTAINER] (--clusterName|-C )"
)

var (
	logsLong = templates.LongDesc(i18n.T(`
        !!!!!clusterName is required strictly!!!!! (--clusterName|-C)
		Print the logs for a container in a pod or specified resource. 
		If the pod has only one container, the container name is optional.`))

	logsExample = templates.Examples(i18n.T(`
		# Return snapshot logs from pod nginx with only one container
		kubectl logs nginx

		# Return snapshot logs from pod nginx with multi containers
		kubectl logs nginx --all-containers=true

		# Return snapshot logs from all containers in pods defined by label app=nginx
		kubectl logs -lapp=nginx --all-containers=true

		# Return snapshot of previous terminated ruby container logs from pod web-1
		kubectl logs -p -c ruby web-1

		# Begin streaming the logs of the ruby container in pod web-1
		kubectl logs -f -c ruby web-1

		# Begin streaming the logs from all containers in pods defined by label app=nginx
		kubectl logs -f -lapp=nginx --all-containers=true

		# Display only the most recent 20 lines of output in pod nginx
		kubectl logs --tail=20 nginx

		# Show all logs from pod nginx written in the last hour
		kubectl logs --since=1h nginx

		# Show logs from a kubelet with an expired serving certificate
		kubectl logs --insecure-skip-tls-verify-backend nginx

		# Return snapshot logs from first container of a job named hello
		kubectl logs job/hello

		# Return snapshot logs from container nginx-1 of a deployment named nginx
		kubectl logs deployment/nginx -c nginx-1`))

	selectorTail    int64 = 10
	logsUsageErrStr       = fmt.Sprintf("expected '%s'.\nPOD or TYPE/NAME is a required argument for the logs command", logsUsageStr)
)

const (
	defaultPodLogsTimeout = 20 * time.Second
)

type LogsOptions struct {
	Namespace     string
	ResourceArg   string
	AllContainers bool
	Options       runtime.Object
	Resources     []string

	ConsumeRequestFn func(rest.ResponseWrapper, io.Writer) error

	// PodLogOptions
	SinceTime                    string
	SinceSeconds                 time.Duration
	Follow                       bool
	Previous                     bool
	Timestamps                   bool
	IgnoreLogErrors              bool
	LimitBytes                   int64
	Tail                         int64
	Container                    string
	InsecureSkipTLSVerifyBackend bool

	// whether or not a container name was given via --container
	ContainerNameSpecified bool
	Selector               string
	MaxFollowConcurrency   int
	Prefix                 bool

	Object           runtime.Object
	GetPodTimeout    time.Duration
	RESTClientGetter genericclioptions.RESTClientGetter
	LogsForObject    polymorphichelpers.LogsForObjectFunc

	genericclioptions.IOStreams

	TailSpecified bool

	containerNameFromRefSpecRegexp *regexp.Regexp

	ClusterName string
	Configs map[string]string
}

func NewLogsOptions(streams genericclioptions.IOStreams, allContainers bool) *LogsOptions {
	return &LogsOptions{
		IOStreams:            streams,
		AllContainers:        allContainers,
		Tail:                 -1,
		MaxFollowConcurrency: 5,

		containerNameFromRefSpecRegexp: regexp.MustCompile(`spec\.(?:initContainers|containers|ephemeralContainers){(.+)}`),
	}
}

// NewCmdLogs creates a new pod logs command
func NewCmdLogs(f cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	o := NewLogsOptions(streams, false)

	cmd := &cobra.Command{
		Use:                   logsUsageStr,
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Print the logs for a container in a pod"),
		Long:                  logsLong,
		Example:               logsExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f, cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.RunLogs())
		},
	}
	o.AddFlags(cmd)
	return cmd
}

func (o *LogsOptions) AddFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&o.AllContainers, "all-containers", o.AllContainers, "Get all containers' logs in the pod(s).")
	cmd.Flags().BoolVarP(&o.Follow, "follow", "f", o.Follow, "Specify if the logs should be streamed.")
	cmd.Flags().BoolVar(&o.Timestamps, "timestamps", o.Timestamps, "Include timestamps on each line in the log output")
	cmd.Flags().Int64Var(&o.LimitBytes, "limit-bytes", o.LimitBytes, "Maximum bytes of logs to return. Defaults to no limit.")
	cmd.Flags().BoolVarP(&o.Previous, "previous", "p", o.Previous, "If true, print the logs for the previous instance of the container in a pod if it exists.")
	cmd.Flags().Int64Var(&o.Tail, "tail", o.Tail, "Lines of recent log file to display. Defaults to -1 with no selector, showing all log lines otherwise 10, if a selector is provided.")
	cmd.Flags().BoolVar(&o.IgnoreLogErrors, "ignore-errors", o.IgnoreLogErrors, "If watching / following pod logs, allow for any errors that occur to be non-fatal")
	cmd.Flags().StringVar(&o.SinceTime, "since-time", o.SinceTime, i18n.T("Only return logs after a specific date (RFC3339). Defaults to all logs. Only one of since-time / since may be used."))
	cmd.Flags().DurationVar(&o.SinceSeconds, "since", o.SinceSeconds, "Only return logs newer than a relative duration like 5s, 2m, or 3h. Defaults to all logs. Only one of since-time / since may be used.")
	cmd.Flags().StringVarP(&o.Container, "container", "c", o.Container, "Print the logs of this container")
	cmd.Flags().BoolVar(&o.InsecureSkipTLSVerifyBackend, "insecure-skip-tls-verify-backend", o.InsecureSkipTLSVerifyBackend,
		"Skip verifying the identity of the kubelet that logs are requested from.  In theory, an attacker could provide invalid log content back. You might want to use this if your kubelet serving certificates have expired.")
	cmdutil.AddPodRunningTimeoutFlag(cmd, defaultPodLogsTimeout)
	cmd.Flags().StringVarP(&o.Selector, "selector", "l", o.Selector, "Selector (label query) to filter on.")
	cmd.Flags().IntVar(&o.MaxFollowConcurrency, "max-log-requests", o.MaxFollowConcurrency, "Specify maximum number of concurrent logs to follow when using by a selector. Defaults to 5.")
	cmd.Flags().BoolVar(&o.Prefix, "prefix", o.Prefix, "Prefix each log line with the log source (pod name and container name)")
	cmdutil.AddClusterVarFlags(cmd, &o.ClusterName, o.ClusterName)
}

func (o *LogsOptions) ToLogOptions() (*corev1.PodLogOptions, error) {
	logOptions := &corev1.PodLogOptions{
		Container:                    o.Container,
		Follow:                       o.Follow,
		Previous:                     o.Previous,
		Timestamps:                   o.Timestamps,
		InsecureSkipTLSVerifyBackend: o.InsecureSkipTLSVerifyBackend,
	}

	if len(o.SinceTime) > 0 {
		t, err := util.ParseRFC3339(o.SinceTime, metav1.Now)
		if err != nil {
			return nil, err
		}

		logOptions.SinceTime = &t
	}

	if o.LimitBytes != 0 {
		logOptions.LimitBytes = &o.LimitBytes
	}

	if o.SinceSeconds != 0 {
		// round up to the nearest second
		sec := int64(o.SinceSeconds.Round(time.Second).Seconds())
		logOptions.SinceSeconds = &sec
	}

	if len(o.Selector) > 0 && o.Tail == -1 && !o.TailSpecified {
		logOptions.TailLines = &selectorTail
	} else if o.Tail != -1 {
		logOptions.TailLines = &o.Tail
	}

	return logOptions, nil
}

func (o *LogsOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	o.ContainerNameSpecified = cmd.Flag("container").Changed
	o.TailSpecified = cmd.Flag("tail").Changed
	o.Resources = args

	switch len(args) {
	case 0:
		if len(o.Selector) == 0 {
			return cmdutil.UsageErrorf(cmd, "%s", logsUsageErrStr)
		}
	case 1:
		o.ResourceArg = args[0]
		if len(o.Selector) != 0 {
			return cmdutil.UsageErrorf(cmd, "only a selector (-l) or a POD name is allowed")
		}
	case 2:
		o.ResourceArg = args[0]
		o.Container = args[1]
	default:
		return cmdutil.UsageErrorf(cmd, "%s", logsUsageErrStr)
	}

	o.Configs = make(map[string]string)
	if len(configs.ClusterName) != len(configs.ConfigContent) {
		return fmt.Errorf("the numbers of ClusterName and the ConfigContent is unmatched")
	}
	if len(configs.ClusterName) == 0 || len(configs.ConfigContent) == 0 {
		return fmt.Errorf("fail to find configs to set")
	}

	for i := 0; i < len(configs.ClusterName); i++ {
		o.Configs[configs.ClusterName[i]] = configs.ConfigContent[i]
	}


	if len(o.ClusterName) > 0 {
		flag := false
		for filename := range o.Configs {
			if filename == o.ClusterName {
				flag = true
				break
			}
		}
		if !flag {
			return fmt.Errorf("the clusterName can not be found")
		}
	} else {
		return fmt.Errorf("Please set the clusterName")
	}

	ClientConfig, _ := f.NewClientConfigFromBytesWithConfigFlags([]byte(o.Configs[o.ClusterName]))
	f.SetClientConfig(&ClientConfig)

	var err error
	o.Namespace, _, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	o.ConsumeRequestFn = DefaultConsumeRequest

	o.GetPodTimeout, err = cmdutil.GetPodRunningTimeoutFlag(cmd)
	if err != nil {
		return err
	}

	o.Options, err = o.ToLogOptions()
	if err != nil {
		return err
	}

	o.RESTClientGetter = f
	o.LogsForObject = polymorphichelpers.LogsForObjectFn

	if o.Object == nil {
		builder := f.NewBuilder().
			WithScheme(scheme.Scheme, scheme.Scheme.PrioritizedVersionsAllGroups()...).
			NamespaceParam(o.Namespace).DefaultNamespace().
			SingleResourceType()
		if o.ResourceArg != "" {
			builder.ResourceNames("pods", o.ResourceArg)
		}
		if o.Selector != "" {
			builder.ResourceTypes("pods").LabelSelectorParam(o.Selector)
		}
		infos, err := builder.Do().Infos()
		if err != nil {
			return err
		}
		if o.Selector == "" && len(infos) != 1 {
			return errors.New("expected a resource")
		}
		o.Object = infos[0].Object
		if o.Selector != "" && len(o.Object.(*corev1.PodList).Items) == 0 {
			fmt.Fprintf(o.ErrOut, "No resources found in %s namespace.\n", o.Namespace)
		}
	}

	return nil
}

func (o LogsOptions) Validate() error {
	if len(o.SinceTime) > 0 && o.SinceSeconds != 0 {
		return fmt.Errorf("at most one of `sinceTime` or `sinceSeconds` may be specified")
	}

	logsOptions, ok := o.Options.(*corev1.PodLogOptions)
	if !ok {
		return errors.New("unexpected logs options object")
	}
	if o.AllContainers && len(logsOptions.Container) > 0 {
		return fmt.Errorf("--all-containers=true should not be specified with container name %s", logsOptions.Container)
	}

	if o.ContainerNameSpecified && len(o.Resources) == 2 {
		return fmt.Errorf("only one of -c or an inline [CONTAINER] arg is allowed")
	}

	if o.LimitBytes < 0 {
		return fmt.Errorf("--limit-bytes must be greater than 0")
	}

	if logsOptions.SinceSeconds != nil && *logsOptions.SinceSeconds < int64(0) {
		return fmt.Errorf("--since must be greater than 0")
	}

	if logsOptions.TailLines != nil && *logsOptions.TailLines < -1 {
		return fmt.Errorf("--tail must be greater than or equal to -1")
	}

	return nil
}

// RunLogs retrieves a pod log
func (o LogsOptions) RunLogs() error {
	requests, err := o.LogsForObject(o.RESTClientGetter, o.Object, o.Options, o.GetPodTimeout, o.AllContainers)
	if err != nil {
		return err
	}

	if o.Follow && len(requests) > 1 {
		if len(requests) > o.MaxFollowConcurrency {
			return fmt.Errorf(
				"you are attempting to follow %d log streams, but maximum allowed concurrency is %d, use --max-log-requests to increase the limit",
				len(requests), o.MaxFollowConcurrency,
			)
		}

		return o.parallelConsumeRequest(requests)
	}

	return o.sequentialConsumeRequest(requests)
}

func (o LogsOptions) parallelConsumeRequest(requests map[corev1.ObjectReference]rest.ResponseWrapper) error {
	reader, writer := io.Pipe()
	wg := &sync.WaitGroup{}
	wg.Add(len(requests))
	for objRef, request := range requests {
		go func(objRef corev1.ObjectReference, request rest.ResponseWrapper) {
			defer wg.Done()
			out := o.addPrefixIfNeeded(objRef, writer)
			if err := o.ConsumeRequestFn(request, out); err != nil {
				if !o.IgnoreLogErrors {
					writer.CloseWithError(err)

					// It's important to return here to propagate the error via the pipe
					return
				}

				fmt.Fprintf(writer, "error: %v\n", err)
			}

		}(objRef, request)
	}

	go func() {
		wg.Wait()
		writer.Close()
	}()

	_, err := io.Copy(o.Out, reader)
	return err
}

func (o LogsOptions) sequentialConsumeRequest(requests map[corev1.ObjectReference]rest.ResponseWrapper) error {
	for objRef, request := range requests {
		out := o.addPrefixIfNeeded(objRef, o.Out)
		if err := o.ConsumeRequestFn(request, out); err != nil {
			if !o.IgnoreLogErrors {
				return err
			}

			fmt.Fprintf(o.Out, "error: %v\n", err)
		}
	}

	return nil
}

func (o LogsOptions) addPrefixIfNeeded(ref corev1.ObjectReference, writer io.Writer) io.Writer {
	if !o.Prefix || ref.FieldPath == "" || ref.Name == "" {
		return writer
	}

	// We rely on ref.FieldPath to contain a reference to a container
	// including a container name (not an index) so we can get a container name
	// without making an extra API request.
	var containerName string
	containerNameMatches := o.containerNameFromRefSpecRegexp.FindStringSubmatch(ref.FieldPath)
	if len(containerNameMatches) == 2 {
		containerName = containerNameMatches[1]
	}

	prefix := fmt.Sprintf("[pod/%s/%s] ", ref.Name, containerName)
	return &prefixingWriter{
		prefix: []byte(prefix),
		writer: writer,
	}
}

// DefaultConsumeRequest reads the data from request and writes into
// the out writer. It buffers data from requests until the newline or io.EOF
// occurs in the data, so it doesn't interleave logs sub-line
// when running concurrently.
//
// A successful read returns err == nil, not err == io.EOF.
// Because the function is defined to read from request until io.EOF, it does
// not treat an io.EOF as an error to be reported.
func DefaultConsumeRequest(request rest.ResponseWrapper, out io.Writer) error {
	readCloser, err := request.Stream(context.TODO())
	if err != nil {
		return err
	}
	defer readCloser.Close()

	r := bufio.NewReader(readCloser)
	for {
		bytes, err := r.ReadBytes('\n')
		if _, err := out.Write(bytes); err != nil {
			return err
		}

		if err != nil {
			if err != io.EOF {
				return err
			}
			return nil
		}
	}
}

type prefixingWriter struct {
	prefix []byte
	writer io.Writer
}

func (pw *prefixingWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	// Perform an "atomic" write of a prefix and p to make sure that it doesn't interleave
	// sub-line when used concurrently with io.PipeWrite.
	n, err := pw.writer.Write(append(pw.prefix, p...))
	if n > len(p) {
		// To comply with the io.Writer interface requirements we must
		// return a number of bytes written from p (0 <= n <= len(p)),
		// so we are ignoring the length of the prefix here.
		return len(p), err
	}
	return n, err
}
