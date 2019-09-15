package annotation_config

import (
	"github.com/derailed/popeye/internal/issues"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type (
	SkipAnnotationConfig map[issues.ID]struct{}
)

func NewSkipAnnotationConfig(meta *metav1.ObjectMeta) SkipAnnotationConfig {
	skipCodes := make(map[issues.ID]struct{})

	skipAnnotationValue, found := meta.Annotations["popeye-skip-sanitizer-codes"]
	if found {
		for _, codeStr := range strings.Split(skipAnnotationValue, ",") {
			code, err := strconv.Atoi(strings.Trim(codeStr, " "))
			if err == nil {
				skipCodes[issues.ID(code)] = struct{}{}
			}
		}
	}

	return skipCodes
}


