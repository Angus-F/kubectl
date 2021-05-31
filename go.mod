module github.com/Angus-F/kubectl

go 1.16

require (
	github.com/Angus-F/cli-runtime v0.0.0-20210526153903-015c3e143216
	github.com/Angus-F/client-go v0.0.0-20210526225611-ccac4151908b
	github.com/MakeNowJust/heredoc v0.0.0-20170808103936-bb23615498cd
	github.com/chai2010/gettext-go v0.0.0-20160711120539-c6fed771bfd5
	github.com/davecgh/go-spew v1.1.1
	github.com/daviddengcn/go-colortext v0.0.0-20160507010035-511bcaf42ccd
	github.com/docker/distribution v2.7.1+incompatible
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d
	github.com/fatih/camelcase v1.0.0
	github.com/fvbommel/sortorder v1.0.1
	github.com/golangplus/testing v0.0.0-20180327235837-af21d9c3145e // indirect
	github.com/google/go-cmp v0.5.4
	github.com/googleapis/gnostic v0.5.1
	github.com/jonboulle/clockwork v0.1.0
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de
	github.com/lithammer/dedent v1.1.0
	github.com/mitchellh/go-wordwrap v1.0.0
	github.com/moby/term v0.0.0-20201216013528-df9cb8a40635
	github.com/onsi/ginkgo v1.14.0
	github.com/onsi/gomega v1.10.1
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/russross/blackfriday v1.5.2
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	golang.org/x/sys v0.0.0-20210426230700-d19ff857e887
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.1
	k8s.io/apimachinery v0.22.0-alpha.2.0.20210526145310-44113beed5d3
	k8s.io/component-base v0.0.0-20210526151053-fd43e6f3a496
	github.com/Angus-F/component-helpers 735649a
	k8s.io/klog/v2 v2.9.0
	k8s.io/kube-openapi v0.0.0-20210421082810-95288971da7e
	github.com/Angus-F/metrics 79b23da
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/kustomize/api v0.8.10
	sigs.k8s.io/kustomize/kustomize/v4 v4.1.3
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/Angus-F/cli-runtime v0.0.0-20210526153903-015c3e143216 => github.com/Angus-F/cli-runtime v0.0.0-20210530083823-b23d0b01184c
	github.com/Angus-F/client-go v0.0.0-20210526225611-ccac4151908b => github.com/Angus-F/client-go v0.0.0-20210530083209-64b6dbd34c98
	k8s.io/api => k8s.io/api v0.0.0-20210526145656-c1965f366614
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20210526145310-44113beed5d3
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20210526145025-2f22fdaa793d
	k8s.io/component-base => k8s.io/component-base v0.0.0-20210526151053-fd43e6f3a496
)
