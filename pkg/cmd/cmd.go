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

package cmd

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Angus-F/client-go/rest"
	"github.com/Angus-F/kubectl/pkg/cmd/cp"
	cmdexec "github.com/Angus-F/kubectl/pkg/cmd/exec"
	"github.com/Angus-F/kubectl/pkg/cmd/logs"
	"github.com/Angus-F/kubectl/pkg/cmd/plugin"
	cmdutil "github.com/Angus-F/kubectl/pkg/cmd/util"
	"github.com/Angus-F/kubectl/pkg/util/i18n"
	"github.com/Angus-F/kubectl/pkg/util/templates"
	"github.com/Angus-F/kubectl/pkg/util/term"
	cliflag "k8s.io/component-base/cli/flag"

	"github.com/Angus-F/cli-runtime/pkg/genericclioptions"
)

const (
	bashCompletionFunc = `# call kubectl get $1,
__kubectl_debug_out()
{
    local cmd="$1"
    __kubectl_debug "${FUNCNAME[1]}: get completion by ${cmd}"
    eval "${cmd} 2>/dev/null"
}

__kubectl_override_flag_list=(--kubeconfig --cluster --user --context --namespace --server -n -s)
__kubectl_override_flags()
{
    local ${__kubectl_override_flag_list[*]##*-} two_word_of of var
    for w in "${words[@]}"; do
        if [ -n "${two_word_of}" ]; then
            eval "${two_word_of##*-}=\"${two_word_of}=\${w}\""
            two_word_of=
            continue
        fi
        for of in "${__kubectl_override_flag_list[@]}"; do
            case "${w}" in
                ${of}=*)
                    eval "${of##*-}=\"${w}\""
                    ;;
                ${of})
                    two_word_of="${of}"
                    ;;
            esac
        done
    done
    for var in "${__kubectl_override_flag_list[@]##*-}"; do
        if eval "test -n \"\$${var}\""; then
            eval "echo -n \${${var}}' '"
        fi
    done
}

__kubectl_config_get_contexts()
{
    __kubectl_parse_config "contexts"
}

__kubectl_config_get_clusters()
{
    __kubectl_parse_config "clusters"
}

__kubectl_config_get_users()
{
    __kubectl_parse_config "users"
}

# $1 has to be "contexts", "clusters" or "users"
__kubectl_parse_config()
{
    local template kubectl_out
    template="{{ range .$1  }}{{ .name }} {{ end }}"
    if kubectl_out=$(__kubectl_debug_out "kubectl config $(__kubectl_override_flags) -o template --template=\"${template}\" view"); then
        COMPREPLY=( $( compgen -W "${kubectl_out[*]}" -- "$cur" ) )
    fi
}

# $1 is the name of resource (required)
# $2 is template string for kubectl get (optional)
__kubectl_parse_get()
{
    local template
    template="${2:-"{{ range .items  }}{{ .metadata.name }} {{ end }}"}"
    local kubectl_out
    if kubectl_out=$(__kubectl_debug_out "kubectl get $(__kubectl_override_flags) -o template --template=\"${template}\" \"$1\""); then
        COMPREPLY+=( $( compgen -W "${kubectl_out[*]}" -- "$cur" ) )
    fi
}

# Same as __kubectl_get_resources (with s) but allows completion for only one resource name.
__kubectl_get_resource()
{
    if [[ ${#nouns[@]} -eq 0 ]]; then
      __kubectl_get_resource_helper "" "$cur"
      return # the return status is that of the last command executed in the function body
    fi
    __kubectl_parse_get "${nouns[${#nouns[@]} -1]}"
}

# Same as __kubectl_get_resource (without s) but allows completion for multiple, comma-separated resource names.
__kubectl_get_resources()
{
    local SEPARATOR=','
    if [[ ${#nouns[@]} -eq 0 ]]; then
      local kubectl_out HEAD TAIL
      HEAD=""
      TAIL="$cur"
      # if SEPARATOR is contained in $cur, e.g. "pod,sec"
      if [[ "$cur" = *${SEPARATOR}* ]] ; then
        # set HEAD to "pod,"
        HEAD="${cur%${SEPARATOR}*}${SEPARATOR}"
        # set TAIL to "sec"
        TAIL="${cur##*${SEPARATOR}}"
      fi
      __kubectl_get_resource_helper "$HEAD" "$TAIL"
      return # the return status is that of the last command executed in the function body
    fi
    __kubectl_parse_get "${nouns[${#nouns[@]} -1]}"
}

__kubectl_get_resource_helper()
{
    local kubectl_out HEAD TAIL
    HEAD="$1"
    TAIL="$2"
    if kubectl_out=$(__kubectl_debug_out "kubectl api-resources $(__kubectl_override_flags) -o name --cached --request-timeout=5s --verbs=get"); then
        COMPREPLY=( $( compgen -P "$HEAD" -W "${kubectl_out[*]}" -- "$TAIL" ) )
        return 0
    fi
    return 1
}

__kubectl_get_resource_namespace()
{
    __kubectl_parse_get "namespace"
}

__kubectl_get_resource_pod()
{
    __kubectl_parse_get "pod"
}

__kubectl_get_resource_rc()
{
    __kubectl_parse_get "rc"
}

__kubectl_get_resource_node()
{
    __kubectl_parse_get "node"
}

__kubectl_get_resource_clusterrole()
{
    __kubectl_parse_get "clusterrole"
}

# $1 is the name of the pod we want to get the list of containers inside
__kubectl_get_containers()
{
    local template
    template="{{ range .spec.initContainers }}{{ .name }} {{end}}{{ range .spec.containers  }}{{ .name }} {{ end }}"
    __kubectl_debug "${FUNCNAME} nouns are ${nouns[*]}"

    local len="${#nouns[@]}"
    if [[ ${len} -ne 1 ]]; then
        return
    fi
    local last=${nouns[${len} -1]}
    local kubectl_out
    if kubectl_out=$(__kubectl_debug_out "kubectl get $(__kubectl_override_flags) -o template --template=\"${template}\" pods \"${last}\""); then
        COMPREPLY=( $( compgen -W "${kubectl_out[*]}" -- "$cur" ) )
    fi
}

# Require both a pod and a container to be specified
__kubectl_require_pod_and_container()
{
    if [[ ${#nouns[@]} -eq 0 ]]; then
        __kubectl_parse_get pods
        return 0
    fi;
    __kubectl_get_containers
    return 0
}

__kubectl_cp()
{
    if [[ $(type -t compopt) = "builtin" ]]; then
        compopt -o nospace
    fi

    case "$cur" in
        /*|[.~]*) # looks like a path
            return
            ;;
        *:*) # TODO: complete remote files in the pod
            return
            ;;
        */*) # complete <namespace>/<pod>
            local template namespace kubectl_out
            template="{{ range .items }}{{ .metadata.namespace }}/{{ .metadata.name }}: {{ end }}"
            namespace="${cur%%/*}"
            if kubectl_out=$(__kubectl_debug_out "kubectl get $(__kubectl_override_flags) --namespace \"${namespace}\" -o template --template=\"${template}\" pods"); then
                COMPREPLY=( $(compgen -W "${kubectl_out[*]}" -- "${cur}") )
            fi
            return
            ;;
        *) # complete namespaces, pods, and filedirs
            __kubectl_parse_get "namespace" "{{ range .items  }}{{ .metadata.name }}/ {{ end }}"
            __kubectl_parse_get "pod" "{{ range .items  }}{{ .metadata.name }}: {{ end }}"
            _filedir
            ;;
    esac
}

__kubectl_custom_func() {
    case ${last_command} in
        kubectl_get)
            __kubectl_get_resources
            return
            ;;
        kubectl_describe | kubectl_delete | kubectl_label | kubectl_edit | kubectl_patch |\
        kubectl_annotate | kubectl_expose | kubectl_scale | kubectl_autoscale | kubectl_taint | kubectl_rollout_* |\
        kubectl_apply_edit-last-applied | kubectl_apply_view-last-applied)
            __kubectl_get_resource
            return
            ;;
        kubectl_logs)
            __kubectl_require_pod_and_container
            return
            ;;
        kubectl_exec | kubectl_port-forward | kubectl_top_pod | kubectl_attach)
            __kubectl_get_resource_pod
            return
            ;;
        kubectl_cordon | kubectl_uncordon | kubectl_drain | kubectl_top_node)
            __kubectl_get_resource_node
            return
            ;;
        kubectl_config_use-context | kubectl_config_rename-context | kubectl_config_delete-context)
            __kubectl_config_get_contexts
            return
            ;;
        kubectl_config_delete-cluster)
            __kubectl_config_get_clusters
            return
            ;;
        kubectl_cp)
            __kubectl_cp
            return
            ;;
        *)
            ;;
    esac
}
`
)

const kubectlCmdHeaders = "KUBECTL_COMMAND_HEADERS"

var (
	bashCompletionFlags = map[string]string{
		"namespace": "__kubectl_get_resource_namespace",
		"context":   "__kubectl_config_get_contexts",
		"cluster":   "__kubectl_config_get_clusters",
		"user":      "__kubectl_config_get_users",
	}
)

// NewDefaultKubectlCommand creates the `kubectl` command with default arguments
func NewDefaultKubectlCommand() *cobra.Command {
	return NewDefaultKubectlCommandWithArgs(NewDefaultPluginHandler(plugin.ValidPluginFilenamePrefixes), os.Args, os.Stdin, os.Stdout, os.Stderr)
}

// NewDefaultKubectlCommandWithArgs creates the `kubectl` command with arguments
func NewDefaultKubectlCommandWithArgs(pluginHandler PluginHandler, args []string, in io.Reader, out, errout io.Writer) *cobra.Command {
	cmd := NewKubectlCommand(in, out, errout)

	if pluginHandler == nil {
		return cmd
	}

	if len(args) > 1 {
		cmdPathPieces := args[1:]

		// only look for suitable extension executables if
		// the specified command does not already exist
		if _, _, err := cmd.Find(cmdPathPieces); err != nil {
			if err := HandlePluginCommand(pluginHandler, cmdPathPieces); err != nil {
				fmt.Fprintf(errout, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	}

	return cmd
}

// PluginHandler is capable of parsing command line arguments
// and performing executable filename lookups to search
// for valid plugin files, and execute found plugins.
type PluginHandler interface {
	// exists at the given filename, or a boolean false.
	// Lookup will iterate over a list of given prefixes
	// in order to recognize valid plugin filenames.
	// The first filepath to match a prefix is returned.
	Lookup(filename string) (string, bool)
	// Execute receives an executable's filepath, a slice
	// of arguments, and a slice of environment variables
	// to relay to the executable.
	Execute(executablePath string, cmdArgs, environment []string) error
}

// DefaultPluginHandler implements PluginHandler
type DefaultPluginHandler struct {
	ValidPrefixes []string
}

// NewDefaultPluginHandler instantiates the DefaultPluginHandler with a list of
// given filename prefixes used to identify valid plugin filenames.
func NewDefaultPluginHandler(validPrefixes []string) *DefaultPluginHandler {
	return &DefaultPluginHandler{
		ValidPrefixes: validPrefixes,
	}
}

// Lookup implements PluginHandler
func (h *DefaultPluginHandler) Lookup(filename string) (string, bool) {
	for _, prefix := range h.ValidPrefixes {
		path, err := exec.LookPath(fmt.Sprintf("%s-%s", prefix, filename))
		if err != nil || len(path) == 0 {
			continue
		}
		return path, true
	}

	return "", false
}

// Execute implements PluginHandler
func (h *DefaultPluginHandler) Execute(executablePath string, cmdArgs, environment []string) error {

	// Windows does not support exec syscall.
	if runtime.GOOS == "windows" {
		cmd := exec.Command(executablePath, cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = environment
		err := cmd.Run()
		if err == nil {
			os.Exit(0)
		}
		return err
	}

	// invoke cmd binary relaying the environment and args given
	// append executablePath to cmdArgs, as execve will make first argument the "binary name".
	return syscall.Exec(executablePath, append([]string{executablePath}, cmdArgs...), environment)
}

// HandlePluginCommand receives a pluginHandler and command-line arguments and attempts to find
// a plugin executable on the PATH that satisfies the given arguments.
func HandlePluginCommand(pluginHandler PluginHandler, cmdArgs []string) error {
	var remainingArgs []string // all "non-flag" arguments
	for _, arg := range cmdArgs {
		if strings.HasPrefix(arg, "-") {
			break
		}
		remainingArgs = append(remainingArgs, strings.Replace(arg, "-", "_", -1))
	}

	if len(remainingArgs) == 0 {
		// the length of cmdArgs is at least 1
		return fmt.Errorf("flags cannot be placed before plugin name: %s", cmdArgs[0])
	}

	foundBinaryPath := ""

	// attempt to find binary, starting at longest possible name with given cmdArgs
	for len(remainingArgs) > 0 {
		path, found := pluginHandler.Lookup(strings.Join(remainingArgs, "-"))
		if !found {
			remainingArgs = remainingArgs[:len(remainingArgs)-1]
			continue
		}

		foundBinaryPath = path
		break
	}

	if len(foundBinaryPath) == 0 {
		return nil
	}

	// invoke cmd binary relaying the current environment and args given
	if err := pluginHandler.Execute(foundBinaryPath, cmdArgs[len(remainingArgs):], os.Environ()); err != nil {
		return err
	}

	return nil
}

// NewKubectlCommand creates the `kubectl` command and its nested children.
func NewKubectlCommand(in io.Reader, out, err io.Writer) *cobra.Command {
	warningHandler := rest.NewWarningWriter(err, rest.WarningWriterOptions{Deduplicate: true, Color: term.AllowsColorOutput(err)})
	warningsAsErrors := false
	// Parent command to which all subcommands are added.
	cmds := &cobra.Command{
		Use:   "kesctl",
		Short: i18n.T("kesctl controls the Kubernetes cluster manager"),
		Long: templates.LongDesc(`
      kesctl controls the Kubernetes cluster manager only for 'exec', 'cp' and 'logs', 
      and this version need user to choose the specific cluster by --clusterName|-C, 
      otherwise it may cause error.`),
		Run: runHelp,
		// Hook before and after Run initialize and write profiles to disk,
		// respectively.
		PersistentPreRunE: func(*cobra.Command, []string) error {
			rest.SetDefaultWarningHandler(warningHandler)
			return initProfiling()
		},
		PersistentPostRunE: func(*cobra.Command, []string) error {
			if err := flushProfiling(); err != nil {
				return err
			}
			if warningsAsErrors {
				count := warningHandler.WarningCount()
				switch count {
				case 0:
					// no warnings
				case 1:
					return fmt.Errorf("%d warning received", count)
				default:
					return fmt.Errorf("%d warnings received", count)
				}
			}
			return nil
		},
		BashCompletionFunction: bashCompletionFunc,
	}

	flags := cmds.PersistentFlags()
	flags.SetNormalizeFunc(cliflag.WarnWordSepNormalizeFunc) // Warn for "_" flags

	// Normalize all flags that are coming from other packages or pre-configurations
	// a.k.a. change all "_" to "-". e.g. glog package
	flags.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)

	addProfilingFlags(flags)

	flags.BoolVar(&warningsAsErrors, "warnings-as-errors", warningsAsErrors, "Treat warnings received from the server as errors and exit with a non-zero exit code")

	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	kubeConfigFlags.AddFlags(flags)
	matchVersionKubeConfigFlags := cmdutil.NewMatchVersionFlags(kubeConfigFlags)
	matchVersionKubeConfigFlags.AddFlags(cmds.PersistentFlags())
	// Updates hooks to add kubectl command headers: SIG CLI KEP 859.
	addCmdHeaderHooks(cmds, kubeConfigFlags)

	cmds.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	f := cmdutil.NewFactory(matchVersionKubeConfigFlags)

	// Sending in 'nil' for the getLanguageFn() results in using
	// the LANG environment variable.
	//
	// TODO: Consider adding a flag or file preference for setting
	// the language, instead of just loading from the LANG env. variable.
	i18n.LoadTranslations("kubectl", nil)

	// From this point and forward we get warnings on flags that contain "_" separators
	cmds.SetGlobalNormalizationFunc(cliflag.WarnWordSepNormalizeFunc)

	ioStreams := genericclioptions.IOStreams{In: in, Out: out, ErrOut: err}

	// Proxy command is incompatible with CommandHeaderRoundTripper, so
	// clear the WrapConfigFn before running proxy command.
	/**
	proxyCmd := proxy.NewCmdProxy(f, ioStreams)
	proxyCmd.PreRun = func(cmd *cobra.Command, args []string) {
		kubeConfigFlags.WrapConfigFn = nil
	}

	 */
	groups := templates.CommandGroups{
		/**{
			Message: "Basic Commands (Beginner):",
			Commands: []*cobra.Command{
				create.NewCmdCreate(f, ioStreams),
				expose.NewCmdExposeService(f, ioStreams),
				run.NewCmdRun(f, ioStreams),
				set.NewCmdSet(f, ioStreams),
			},
		},
		{
			Message: "Basic Commands (Intermediate):",
			Commands: []*cobra.Command{
				explain.NewCmdExplain("kubectl", f, ioStreams),
				get.NewCmdGet("kubectl", f, ioStreams),
				edit.NewCmdEdit(f, ioStreams),
				delete.NewCmdDelete(f, ioStreams),
			},
		},
		{
			Message: "Deploy Commands:",
			Commands: []*cobra.Command{
				rollout.NewCmdRollout(f, ioStreams),
				scale.NewCmdScale(f, ioStreams),
				autoscale.NewCmdAutoscale(f, ioStreams),
			},
		},
		{
			Message: "Cluster Management Commands:",
			Commands: []*cobra.Command{
				certificates.NewCmdCertificate(f, ioStreams),
				clusterinfo.NewCmdClusterInfo(f, ioStreams),
				top.NewCmdTop(f, ioStreams),
				drain.NewCmdCordon(f, ioStreams),
				drain.NewCmdUncordon(f, ioStreams),
				drain.NewCmdDrain(f, ioStreams),
				taint.NewCmdTaint(f, ioStreams),
			},
		},*/
		{
			Message: "Troubleshooting and Debugging Commands:",
			Commands: []*cobra.Command{
				//describe.NewCmdDescribe("kubectl", f, ioStreams),
				logs.NewCmdLogs(f, ioStreams),
				//attach.NewCmdAttach(f, ioStreams),
				cmdexec.NewCmdExec(f, ioStreams),
				//portforward.NewCmdPortForward(f, ioStreams),
				//proxyCmd,
				cp.NewCmdCp(f, ioStreams),
				//auth.NewCmdAuth(f, ioStreams),
				//debug.NewCmdDebug(f, ioStreams),
			},
		},
		/**
		{
			Message: "Advanced Commands:",
			Commands: []*cobra.Command{
				diff.NewCmdDiff(f, ioStreams),
				apply.NewCmdApply("kubectl", f, ioStreams),
				patch.NewCmdPatch(f, ioStreams),
				replace.NewCmdReplace(f, ioStreams),
				wait.NewCmdWait(f, ioStreams),
				kustomize.NewCmdKustomize(ioStreams),
			},
		},
		{
			Message: "Settings Commands:",
			Commands: []*cobra.Command{
				label.NewCmdLabel(f, ioStreams),
				annotate.NewCmdAnnotate("kubectl", f, ioStreams),
				completion.NewCmdCompletion(ioStreams.Out, ""),
			},
		},
		*/
	}
	groups.Add(cmds)

	filters := []string{"options"}

	// Hide the "alpha" subcommand if there are no alpha commands in this build.
	/**
	alpha := NewCmdAlpha(f, ioStreams)
	if !alpha.HasSubCommands() {
		filters = append(filters, alpha.Name())
	}
	 */
	templates.ActsAsRootCommand(cmds, filters, groups...)

	for name, completion := range bashCompletionFlags {
		if cmds.Flag(name) != nil {
			if cmds.Flag(name).Annotations == nil {
				cmds.Flag(name).Annotations = map[string][]string{}
			}
			cmds.Flag(name).Annotations[cobra.BashCompCustom] = append(
				cmds.Flag(name).Annotations[cobra.BashCompCustom],
				completion,
			)
		}
	}
/**
	cmds.AddCommand(alpha)
	cmds.AddCommand(cmdconfig.NewCmdConfig(f, clientcmd.NewDefaultPathOptions(), ioStreams))
	cmds.AddCommand(plugin.NewCmdPlugin(f, ioStreams))
	cmds.AddCommand(version.NewCmdVersion(f, ioStreams))
	cmds.AddCommand(apiresources.NewCmdAPIVersions(f, ioStreams))
	cmds.AddCommand(apiresources.NewCmdAPIResources(f, ioStreams))
	cmds.AddCommand(options.NewCmdOptions(ioStreams.Out))

 */

	return cmds
}

// addCmdHeaderHooks performs updates on two hooks:
//   1) Modifies the passed "cmds" persistent pre-run function to parse command headers.
//      These headers will be subsequently added as X-headers to every
//      REST call.
//   2) Adds CommandHeaderRoundTripper as a wrapper around the standard
//      RoundTripper. CommandHeaderRoundTripper adds X-Headers then delegates
//      to standard RoundTripper.
// For alpha, these hooks are only updated if the KUBECTL_COMMAND_HEADERS
// environment variable is set.
// See SIG CLI KEP 859 for more information:
//   https://github.com/kubernetes/enhancements/tree/master/keps/sig-cli/859-kubectl-headers
func addCmdHeaderHooks(cmds *cobra.Command, kubeConfigFlags *genericclioptions.ConfigFlags) {
	if _, exists := os.LookupEnv(kubectlCmdHeaders); !exists {
		return
	}
	crt := &genericclioptions.CommandHeaderRoundTripper{}
	existingPreRunE := cmds.PersistentPreRunE
	// Add command parsing to the existing persistent pre-run function.
	cmds.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		crt.ParseCommandHeaders(cmd, args)
		return existingPreRunE(cmd, args)
	}
	// Wraps CommandHeaderRoundTripper around standard RoundTripper.
	kubeConfigFlags.WrapConfigFn = func(c *rest.Config) *rest.Config {
		c.Wrap(func(rt http.RoundTripper) http.RoundTripper {
			crt.Delegate = rt
			return crt
		})
		return c
	}
}

func runHelp(cmd *cobra.Command, args []string) {
	cmd.Help()
}
