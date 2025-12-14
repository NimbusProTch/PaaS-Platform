package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
	"gopkg.in/yaml.v3"
)

// InfraForgeRequest represents the new InfraForge CRD structure
type InfraForgeRequest struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec InfraForgeSpec `json:"spec"`
}

type InfraForgeSpec struct {
	Tenant      string         `json:"tenant"`      // finance, marketing, etc.
	Environment string         `json:"environment"` // dev, test, uat
	Business    []ServiceItem  `json:"business"`    // Business applications
	Platform    []ServiceItem  `json:"platform"`    // Platform services (vault, istio)
	Operators   []ServiceItem  `json:"operators"`   // Database operators
}

type ServiceItem struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Profile string `json:"profile,omitempty"` // dev/standard/production
}

type InfraForgeProcessor struct {
	request      *InfraForgeRequest
	outputDir    string
	gitRepoURL   string
	gitBranch    string
}

func NewInfraForgeProcessor(request *InfraForgeRequest, outputDir string) *InfraForgeProcessor {
	// Get Git configuration from environment or use defaults
	gitRepoURL := os.Getenv("GIT_REPO_URL")
	if gitRepoURL == "" {
		gitRepoURL = "https://github.com/NimbusProTch/PaaS-Platform.git"
	}

	gitBranch := os.Getenv("GIT_BRANCH")
	if gitBranch == "" {
		gitBranch = "main"
	}

	return &InfraForgeProcessor{
		request:    request,
		outputDir:  outputDir,
		gitRepoURL: gitRepoURL,
		gitBranch:  gitBranch,
	}
}

func (p *InfraForgeProcessor) Process() error {
	// 1. Generate .kratix metadata
	if err := p.generateKratixMetadata(); err != nil {
		return fmt.Errorf("failed to generate kratix metadata: %w", err)
	}
	
	// 2. Generate ArgoCD projects
	if err := p.generateArgoProjects(); err != nil {
		return fmt.Errorf("failed to generate ArgoCD projects: %w", err)
	}
	
	// 3. Generate Root Application (only for first deployment)
	// Disabled - root app should be created manually during platform setup
	// if err := p.generateRootApplication(); err != nil {
	// 	return fmt.Errorf("failed to generate root application: %w", err)
	// }
	
	// 4. Generate ApplicationSets
	if err := p.generateApplicationSets(); err != nil {
		return fmt.Errorf("failed to generate application sets: %w", err)
	}
	
	// 5. Generate Operators
	if err := p.generateOperators(); err != nil {
		return fmt.Errorf("failed to generate operators: %w", err)
	}
	
	// 6. Generate Platform Services
	if err := p.generatePlatformServices(); err != nil {
		return fmt.Errorf("failed to generate platform services: %w", err)
	}
	
	// 7. Generate Business Applications
	if err := p.generateBusinessApplications(); err != nil {
		return fmt.Errorf("failed to generate business applications: %w", err)
	}
	
	return nil
}

func (p *InfraForgeProcessor) generateKratixMetadata() error {
	kratixDir := filepath.Join(p.outputDir, ".kratix")
	if err := os.MkdirAll(kratixDir, 0755); err != nil {
		return err
	}
	
	// Generate simple metadata filename
	metadataFile := filepath.Join(kratixDir, fmt.Sprintf("%s-%s-nonprod.yaml", 
		p.request.Spec.Tenant, p.request.Spec.Environment))
	
	metadata := map[string]interface{}{
		"apiVersion": "platform.kratix.io/v1alpha1",
		"kind":       "Work",
		"metadata": map[string]interface{}{
			"name":      fmt.Sprintf("%s-%s", p.request.Spec.Tenant, p.request.Spec.Environment),
			"namespace": "kratix-platform-system",
		},
		"spec": map[string]interface{}{
			"tenant":      p.request.Spec.Tenant,
			"environment": p.request.Spec.Environment,
			"generated":   time.Now().Format(time.RFC3339),
		},
	}
	
	return p.writeYAML(metadataFile, metadata)
}

func (p *InfraForgeProcessor) generateArgoProjects() error {
	env := p.request.Spec.Environment
	projectDir := filepath.Join(p.outputDir, "argocd", env)
	
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return err
	}
	
	projectFile := filepath.Join(projectDir, "project.yaml")
	
	
	project := map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "AppProject",
		"metadata": map[string]interface{}{
			"name":      fmt.Sprintf("infraforge-%s", env),
			"namespace": "infraforge-argocd",
		},
		"spec": map[string]interface{}{
			"description": fmt.Sprintf("InfraForge %s Environment", strings.Title(env)),
			"sourceRepos": []string{"*"},
			"destinations": []map[string]interface{}{
				{
					"namespace": "*",
					"server":    "https://kubernetes.default.svc",
				},
			},
			"clusterResourceWhitelist": []map[string]interface{}{
				{
					"group": "*",
					"kind":  "*",
				},
			},
			"namespaceResourceWhitelist": []map[string]interface{}{
				{
					"group": "*",
					"kind":  "*",
				},
			},
			"roles": p.generateProjectRoles(env),
		},
	}
	
	return p.writeYAML(projectFile, project)
}

func (p *InfraForgeProcessor) generateProjectRoles(env string) []map[string]interface{} {
	// Production has restricted windows
	if env == "prod" {
		return []map[string]interface{}{
			{
				"name": "admin",
				"policies": []string{
					fmt.Sprintf("p, proj:infraforge-%s:admin, applications, *, infraforge-%s/*, allow", env, env),
				},
			},
			{
				"name": "developer",
				"policies": []string{
					fmt.Sprintf("p, proj:infraforge-%s:developer, applications, get, infraforge-%s/*, allow", env, env),
					fmt.Sprintf("p, proj:infraforge-%s:developer, applications, sync, infraforge-%s/*, allow, * * 6-14 * 1-5", env, env),
				},
			},
		}
	}
	
	// Dev/Test/UAT have unrestricted access
	return []map[string]interface{}{
		{
			"name": "admin",
			"policies": []string{
				fmt.Sprintf("p, proj:infraforge-%s:admin, applications, *, infraforge-%s/*, allow", env, env),
			},
		},
		{
			"name": "developer", 
			"policies": []string{
				fmt.Sprintf("p, proj:infraforge-%s:developer, applications, *, infraforge-%s/*, allow", env, env),
			},
		},
	}
}

func (p *InfraForgeProcessor) generateRootApplication() error {
	rootDir := filepath.Join(p.outputDir, "infraforge-nonprod-root-app")
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return err
	}
	
	rootAppFile := filepath.Join(rootDir, "nonprod-root-app.yaml")
	
	rootApp := map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]interface{}{
			"name":      "infraforge-nonprod-root",
			"namespace": "infraforge-argocd",
			"finalizers": []string{"resources-finalizer.argocd.argoproj.io"},
		},
		"spec": map[string]interface{}{
			"project": "default",
			"source": map[string]interface{}{
				"repoURL":        p.gitRepoURL,
				"targetRevision": p.gitBranch,
				"path":           "manifests/appsets/dev",
			},
			"destination": map[string]interface{}{
				"server":    "https://kubernetes.default.svc",
				"namespace": "infraforge-argocd",
			},
			"syncPolicy": map[string]interface{}{
				"automated": map[string]interface{}{
					"prune":    true,
					"selfHeal": true,
				},
				"syncOptions": []string{"CreateNamespace=true"},
			},
		},
	}
	
	return p.writeYAML(rootAppFile, rootApp)
}

func (p *InfraForgeProcessor) generateApplicationSets() error {
	env := p.request.Spec.Environment
	appsetDir := filepath.Join(p.outputDir, "appsets", env)
	
	if err := os.MkdirAll(appsetDir, 0755); err != nil {
		return err
	}
	
	// 1. Business AppSet
	if err := p.generateAppSet(appsetDir, "business", env); err != nil {
		return err
	}
	
	// 2. Platform AppSet
	if err := p.generateAppSet(appsetDir, "platform", env); err != nil {
		return err
	}
	
	// 3. Operators AppSet
	if err := p.generateAppSet(appsetDir, "operator", env); err != nil {
		return err
	}
	
	return nil
}

func (p *InfraForgeProcessor) generateAppSet(dir, appType, env string) error {
	appsetFile := filepath.Join(dir, fmt.Sprintf("%s-appset.yaml", appType))
	
	var path string
	var namespace string
	
	switch appType {
	case "business":
		path = fmt.Sprintf("manifests/apps/%s/business-apps/*", env)
		namespace = fmt.Sprintf("%s-%s", p.request.Spec.Tenant, env)
	case "platform":
		path = fmt.Sprintf("manifests/apps/%s/platform-apps/*", env)
		namespace = "{{.path.basename}}"
	case "operator":
		path = fmt.Sprintf("manifests/operators/%s/*", env)
		namespace = "infraforge-operators"
	}
	
	appset := map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "ApplicationSet",
		"metadata": map[string]interface{}{
			"name":      fmt.Sprintf("%s-%s-appset", env, appType),
			"namespace": "infraforge-argocd",
		},
		"spec": map[string]interface{}{
			"generators": []map[string]interface{}{
				{
					"git": map[string]interface{}{
						"repoURL":  p.gitRepoURL,
						"revision": p.gitBranch,
						"directories": []map[string]interface{}{
							{"path": path},
						},
					},
				},
			},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      fmt.Sprintf("%s-%s-{{.path.basenameNormalized}}", env, appType),
					"namespace": "infraforge-argocd",
				},
				"spec": map[string]interface{}{
					"project": fmt.Sprintf("infraforge-%s", env),
					"source": map[string]interface{}{
						"repoURL":        p.gitRepoURL,
						"targetRevision": p.gitBranch,
						"path":           "{{.path.path}}",
					},
					"destination": map[string]interface{}{
						"server":    "https://kubernetes.default.svc",
						"namespace": namespace,
					},
					"syncPolicy": map[string]interface{}{
						"automated": map[string]interface{}{
							"prune":    true,
							"selfHeal": true,
						},
						"syncOptions": []string{"CreateNamespace=true"},
					},
				},
			},
		},
	}
	
	return p.writeYAML(appsetFile, appset)
}

func (p *InfraForgeProcessor) generateOperators() error {
	env := p.request.Spec.Environment
	
	for _, op := range p.request.Spec.Operators {
		if !op.Enabled {
			continue
		}
		
		operatorDir := filepath.Join(p.outputDir, "operators", env, op.Name)
		if err := os.MkdirAll(operatorDir, 0755); err != nil {
			return err
		}
		
		// Generate operator installation based on type
		switch op.Name {
		case "redis":
			if err := p.generateRedisOperator(operatorDir); err != nil {
				return err
			}
		case "postgresql":
			if err := p.generatePostgreSQLOperator(operatorDir); err != nil {
				return err
			}
		// Add more operators as needed
		}
	}
	
	return nil
}

func (p *InfraForgeProcessor) generateRedisOperator(dir string) error {
	// Generate Redis CR for the specific tenant/environment
	crFile := filepath.Join(dir, "redis-instance.yaml")
	
	namespace := fmt.Sprintf("%s-%s", p.request.Spec.Tenant, p.request.Spec.Environment)
	
	// Create Redis instance CR
	redisCR := map[string]interface{}{
		"apiVersion": "databases.spotahome.com/v1",
		"kind":       "RedisFailover",
		"metadata": map[string]interface{}{
			"name":      fmt.Sprintf("%s-redis", p.request.Spec.Tenant),
			"namespace": namespace,
			"labels": map[string]interface{}{
				"tenant":      p.request.Spec.Tenant,
				"environment": p.request.Spec.Environment,
				"managed-by":  "infraforge",
			},
		},
		"spec": map[string]interface{}{
			"sentinel": map[string]interface{}{
				"replicas": 3,
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"cpu":    "100m",
						"memory": "100Mi",
					},
				},
			},
			"redis": map[string]interface{}{
				"replicas": 3,
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"cpu":    "100m",
						"memory": "100Mi",
					},
					"limits": map[string]interface{}{
						"cpu":    "400m",
						"memory": "500Mi",
					},
				},
			},
		},
	}
	
	return p.writeYAML(crFile, redisCR)
}

func (p *InfraForgeProcessor) generatePostgreSQLOperator(dir string) error {
	// Generate PostgreSQL Cluster CR
	crFile := filepath.Join(dir, "postgresql-cluster.yaml")
	
	namespace := fmt.Sprintf("%s-%s", p.request.Spec.Tenant, p.request.Spec.Environment)
	
	pgCluster := map[string]interface{}{
		"apiVersion": "postgresql.cnpg.io/v1",
		"kind":       "Cluster",
		"metadata": map[string]interface{}{
			"name":      fmt.Sprintf("%s-postgres", p.request.Spec.Tenant),
			"namespace": namespace,
			"labels": map[string]interface{}{
				"tenant":      p.request.Spec.Tenant,
				"environment": p.request.Spec.Environment,
				"managed-by":  "infraforge",
			},
		},
		"spec": map[string]interface{}{
			"instances": 3,
			"primaryUpdateStrategy": "unsupervised",
			
			"postgresql": map[string]interface{}{
				"parameters": map[string]interface{}{
					"max_connections": "200",
					"shared_buffers": "256MB",
					"effective_cache_size": "1GB",
				},
			},
			
			"bootstrap": map[string]interface{}{
				"initdb": map[string]interface{}{
					"database": p.request.Spec.Tenant,
					"owner":    p.request.Spec.Tenant,
				},
			},
			
			"storage": map[string]interface{}{
				"size": "10Gi",
				"storageClass": "standard",
			},
			
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{
					"cpu":    "500m",
					"memory": "1Gi",
				},
				"limits": map[string]interface{}{
					"cpu":    "1",
					"memory": "2Gi",
				},
			},
		},
	}
	
	return p.writeYAML(crFile, pgCluster)
}

func (p *InfraForgeProcessor) generatePlatformServices() error {
	env := p.request.Spec.Environment
	
	for _, svc := range p.request.Spec.Platform {
		if !svc.Enabled {
			continue
		}
		
		svcDir := filepath.Join(p.outputDir, "apps", env, "platform-apps", svc.Name)
		if err := os.MkdirAll(svcDir, 0755); err != nil {
			return err
		}
		
		switch svc.Name {
		case "vault":
			if err := p.generateVaultService(svcDir); err != nil {
				return err
			}
		case "istio":
			if err := p.generateIstioService(svcDir); err != nil {
				return err
			}
		}
	}
	
	return nil
}

func (p *InfraForgeProcessor) generateVaultService(dir string) error {
	valuesFile := filepath.Join(dir, "values.yaml")
	chartFile := filepath.Join(dir, "Chart.yaml")
	
	// Find the profile from platform services
	profile := "dev"
	for _, svc := range p.request.Spec.Platform {
		if svc.Name == "vault" && svc.Profile != "" {
			profile = svc.Profile
			break
		}
	}
	
	// Load profile values
	profileData, err := p.loadProfileValues("vault", profile)
	if err != nil {
		return fmt.Errorf("failed to load vault profile: %w", err)
	}
	
	// Generate Helm values from template
	valuesData := map[string]interface{}{
		"Name":        p.request.Metadata.Name,
		"Tenant":      p.request.Spec.Tenant,
		"Environment": p.request.Spec.Environment,
		"Profile":     profile,
		"Values":      profileData,
	}
	
	helmValues, err := p.renderTemplate("vault", "helm-values.tmpl", valuesData)
	if err != nil {
		return fmt.Errorf("failed to render vault helm values: %w", err)
	}
	
	// Write values.yaml
	if err := os.WriteFile(valuesFile, []byte(helmValues), 0644); err != nil {
		return fmt.Errorf("failed to write values.yaml: %w", err)
	}
	
	// Create Chart.yaml pointing to upstream Vault chart
	chart := map[string]interface{}{
		"apiVersion": "v2",
		"name":       "vault",
		"description": "HashiCorp Vault for " + p.request.Spec.Tenant + "-" + p.request.Spec.Environment,
		"type":       "application",
		"version":    "0.1.0",
		"dependencies": []map[string]interface{}{
			{
				"name":       "vault",
				"version":    "0.25.0",
				"repository": "https://helm.releases.hashicorp.com",
			},
		},
	}
	
	return p.writeYAML(chartFile, chart)
}

func (p *InfraForgeProcessor) getVaultValues() string {
	if p.request.Spec.Environment == "dev" {
		return `server:
  dev:
    enabled: true
  standalone:
    enabled: true
  dataStorage:
    size: 1Gi`
	}
	
	// Production values
	return `server:
  ha:
    enabled: true
    replicas: 3
  dataStorage:
    size: 10Gi
  resources:
    requests:
      memory: 256Mi
      cpu: 250m
    limits:
      memory: 512Mi
      cpu: 500m`
}

func (p *InfraForgeProcessor) generateIstioService(dir string) error {
	// Similar to Vault but for Istio
	return nil
}

func (p *InfraForgeProcessor) generateBusinessApplications() error {
	env := p.request.Spec.Environment
	
	for _, app := range p.request.Spec.Business {
		if !app.Enabled {
			continue
		}
		
		appDir := filepath.Join(p.outputDir, "apps", env, "business-apps", app.Name)
		if err := os.MkdirAll(appDir, 0755); err != nil {
			return err
		}
		
		// For business apps, we only generate values.yaml and Chart.yaml
		if err := p.generateBusinessAppHelm(appDir, app.Name, app.Profile); err != nil {
			return fmt.Errorf("failed to generate business app %s: %w", app.Name, err)
		}
	}
	
	return nil
}

func (p *InfraForgeProcessor) generateBusinessAppHelm(dir, appName, profile string) error {
	if profile == "" {
		profile = "dev"
	}
	
	// Create Chart.yaml
	chartFile := filepath.Join(dir, "Chart.yaml")
	chart := map[string]interface{}{
		"apiVersion": "v2",
		"name":       appName,
		"description": fmt.Sprintf("%s application for %s-%s", appName, p.request.Spec.Tenant, p.request.Spec.Environment),
		"type":       "application",
		"version":    "0.1.0",
	}
	
	if err := p.writeYAML(chartFile, chart); err != nil {
		return fmt.Errorf("failed to write Chart.yaml: %w", err)
	}
	
	// Create values.yaml based on profile
	valuesFile := filepath.Join(dir, "values.yaml")
	values := map[string]interface{}{
		"nameOverride":     appName,
		"fullnameOverride": appName,
		"tenant":           p.request.Spec.Tenant,
		"environment":      p.request.Spec.Environment,
		"namespace":        fmt.Sprintf("%s-%s", p.request.Spec.Tenant, p.request.Spec.Environment),
		
		"image": map[string]interface{}{
			"repository": "nginx",
			"pullPolicy": "IfNotPresent",
			"tag":        "1.25-alpine",
		},
		
		"service": map[string]interface{}{
			"type": "ClusterIP",
			"port": 80,
		},
		
		"ingress": map[string]interface{}{
			"enabled":         true,
			"className":       "nginx",
			"host":            fmt.Sprintf("%s.%s.local", appName, p.request.Spec.Tenant),
			"tls":             false,
		},
	}
	
	// Add profile-specific values
	switch profile {
	case "production":
		values["replicaCount"] = 3
		values["resources"] = map[string]interface{}{
			"limits": map[string]interface{}{
				"cpu":    "500m",
				"memory": "512Mi",
			},
			"requests": map[string]interface{}{
				"cpu":    "250m",
				"memory": "256Mi",
			},
		}
		values["autoscaling"] = map[string]interface{}{
			"enabled":     true,
			"minReplicas": 3,
			"maxReplicas": 10,
			"targetCPU":   80,
		}
	case "standard":
		values["replicaCount"] = 2
		values["resources"] = map[string]interface{}{
			"limits": map[string]interface{}{
				"cpu":    "200m",
				"memory": "256Mi",
			},
			"requests": map[string]interface{}{
				"cpu":    "100m",
				"memory": "128Mi",
			},
		}
	default: // dev
		values["replicaCount"] = 1
		values["resources"] = map[string]interface{}{
			"limits": map[string]interface{}{
				"cpu":    "100m",
				"memory": "128Mi",
			},
			"requests": map[string]interface{}{
				"cpu":    "50m",
				"memory": "64Mi",
			},
		}
	}
	
	// Write values.yaml
	return p.writeYAML(valuesFile, values)
}


func (p *InfraForgeProcessor) writeYAML(filename string, data interface{}) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2)
	return encoder.Encode(data)
}

func (p *InfraForgeProcessor) loadProfileValues(service, profile string) (map[string]interface{}, error) {
	profilePath := fmt.Sprintf("/platform-templates/%s/profiles/%s.yaml", service, profile)
	
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile %s: %w", profilePath, err)
	}
	
	var values map[string]interface{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("failed to parse profile: %w", err)
	}
	
	return values, nil
}

func (p *InfraForgeProcessor) renderTemplate(service, templateName string, data interface{}) (string, error) {
	templatePath := fmt.Sprintf("/platform-templates/%s/%s", service, templateName)
	
	tmplData, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template: %w", err)
	}
	
	tmpl, err := template.New(templateName).Parse(string(tmplData))
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}
	
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	
	return buf.String(), nil
}

func (p *InfraForgeProcessor) generateFromTemplate(service, templateName, outputPath string, data interface{}) error {
	content, err := p.renderTemplate(service, templateName, data)
	if err != nil {
		return err
	}
	
	return os.WriteFile(outputPath, []byte(content), 0644)
}