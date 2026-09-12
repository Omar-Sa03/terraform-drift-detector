package state

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/medom/terraform-drift-detector/internal/model"
)

type rawState struct {
	Version   int           `json:"version"`
	Resources []rawResource `json:"resources"`
	Modules   []rawV3Module `json:"modules"`
}

type rawResource struct {
	Module    string        `json:"module"`
	Mode      string        `json:"mode"`
	Type      string        `json:"type"`
	Name      string        `json:"name"`
	Provider  string        `json:"provider"`
	Instances []rawInstance `json:"instances"`
}

type rawInstance struct {
	IndexKey   any            `json:"index_key"`
	Attributes map[string]any `json:"attributes"`
}

type rawV3Module struct {
	Path      []string                 `json:"path"`
	Resources map[string]rawV3Resource `json:"resources"`
}

type rawV3Resource struct {
	Type    string       `json:"type"`
	Primary rawV3Primary `json:"primary"`
}

type rawV3Primary struct {
	ID         string            `json:"id"`
	Attributes map[string]string `json:"attributes"`
}

// Parse decodes Terraform state JSON (v3 or v4) into normalized resources.
func Parse(data []byte) ([]model.Resource, error) {
	var st rawState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse terraform state: %w", err)
	}
	switch st.Version {
	case 4:
		return parseV4(st.Resources)
	case 3:
		return parseV3(st.Modules)
	default:
		if len(st.Resources) > 0 {
			return parseV4(st.Resources)
		}
		if len(st.Modules) > 0 {
			return parseV3(st.Modules)
		}
		return nil, fmt.Errorf("unsupported terraform state version %d", st.Version)
	}
}

func parseV4(resources []rawResource) ([]model.Resource, error) {
	out := make([]model.Resource, 0)
	for _, r := range resources {
		if r.Mode != "" && r.Mode != "managed" {
			continue
		}
		for _, inst := range r.Instances {
			attrs := inst.Attributes
			if attrs == nil {
				attrs = map[string]any{}
			}
			addr := resourceAddress(r.Module, r.Type, r.Name, inst.IndexKey)
			out = append(out, normalize(r.Type, r.Provider, addr, attrs))
		}
	}
	return out, nil
}

func parseV3(modules []rawV3Module) ([]model.Resource, error) {
	out := make([]model.Resource, 0)
	for _, mod := range modules {
		prefix := ""
		if len(mod.Path) > 1 {
			parts := make([]string, 0, len(mod.Path)-1)
			for _, p := range mod.Path[1:] {
				parts = append(parts, "module."+p)
			}
			prefix = strings.Join(parts, ".")
		}
		for key, r := range mod.Resources {
			if strings.HasPrefix(key, "data.") {
				continue
			}
			attrs := flattenV3Attrs(r.Primary.Attributes)
			typ := r.Type
			name := key
			if strings.HasPrefix(key, typ+".") {
				name = strings.TrimPrefix(key, typ+".")
			}
			addr := resourceAddress(prefix, typ, name, nil)
			if r.Primary.ID != "" && attrs["id"] == nil {
				attrs["id"] = r.Primary.ID
			}
			out = append(out, normalize(typ, "", addr, attrs))
		}
	}
	return out, nil
}

func flattenV3Attrs(in map[string]string) map[string]any {
	out := map[string]any{}
	tags := map[string]string{}
	for k, v := range in {
		if strings.HasPrefix(k, "tags.") && k != "tags.%" {
			tags[strings.TrimPrefix(k, "tags.")] = v
			continue
		}
		if strings.Contains(k, ".") || strings.HasSuffix(k, "%") || strings.HasSuffix(k, "#") {
			continue
		}
		out[k] = v
	}
	if len(tags) > 0 {
		out["tags"] = tags
	}
	return out
}

func resourceAddress(module, typ, name string, index any) string {
	addr := typ + "." + name
	if module != "" {
		addr = module + "." + addr
	}
	if index == nil {
		return addr
	}
	switch v := index.(type) {
	case string:
		return addr + `["` + v + `"]`
	case float64:
		return addr + "[" + strconv.Itoa(int(v)) + "]"
	default:
		return addr + "[" + fmt.Sprint(v) + "]"
	}
}

func normalize(tfType, providerRaw, address string, attrs map[string]any) model.Resource {
	cloned := cloneAttrs(attrs)
	tags := extractTags(cloned)
	delete(cloned, "tags")
	delete(cloned, "tags_all")
	id := stringAttr(cloned, "id")
	if id == "" {
		id = fallbackID(tfType, cloned)
	}
	return model.Resource{
		Address:    address,
		Type:       tfType,
		Provider:   ProviderFromType(tfType, providerRaw),
		ID:         id,
		Region:     stringAttr(cloned, "region"),
		Attributes: cloned,
		Tags:       tags,
	}
}

func fallbackID(tfType string, attrs map[string]any) string {
	switch tfType {
	case "aws_s3_bucket":
		return stringAttr(attrs, "bucket")
	case "aws_iam_role":
		return stringAttr(attrs, "name")
	default:
		return ""
	}
}

func extractTags(attrs map[string]any) map[string]string {
	tags := map[string]string{}
	mergeTagMap(tags, attrs["tags_all"])
	mergeTagMap(tags, attrs["tags"])
	return tags
}

func mergeTagMap(dst map[string]string, raw any) {
	switch v := raw.(type) {
	case map[string]string:
		for k, val := range v {
			dst[k] = val
		}
	case map[string]any:
		for k, val := range v {
			if val == nil {
				continue
			}
			dst[k] = fmt.Sprint(val)
		}
	}
}

func stringAttr(attrs map[string]any, key string) string {
	v, ok := attrs[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func cloneAttrs(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ProviderFromType infers aws|azure|gcp from a Terraform resource type or provider string.
func ProviderFromType(tfType, providerRaw string) string {
	p := strings.ToLower(providerRaw)
	switch {
	case strings.Contains(p, "azurerm") || strings.Contains(p, "azure"):
		return model.ProviderAzure
	case strings.Contains(p, "google") || strings.Contains(p, "gcp"):
		return model.ProviderGCP
	case strings.Contains(p, "aws"):
		return model.ProviderAWS
	}
	switch {
	case strings.HasPrefix(tfType, "aws_"):
		return model.ProviderAWS
	case strings.HasPrefix(tfType, "azurerm_"):
		return model.ProviderAzure
	case strings.HasPrefix(tfType, "google_"):
		return model.ProviderGCP
	default:
		return "unknown"
	}
}
