package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gaskin/go-platform-generator/pkg/pipeline"
	"gopkg.in/yaml.v3"
)

func main() {
	log.Println("Starting InfraForge Platform Generator...")

	// Read input from Kratix
	inputPath := "/kratix/input/object.yaml"
	if envPath := os.Getenv("KRATIX_INPUT_PATH"); envPath != "" {
		inputPath = envPath
	}

	// Read the input
	data, err := os.ReadFile(inputPath)
	if err != nil {
		log.Fatalf("Failed to read input: %v", err)
	}

	// Check the Kind to determine request type
	var rawRequest map[string]interface{}
	if err := yaml.Unmarshal(data, &rawRequest); err != nil {
		log.Fatalf("Failed to parse raw request: %v", err)
	}

	outputDir := "/kratix/output"
	if envDir := os.Getenv("KRATIX_OUTPUT_PATH"); envDir != "" {
		outputDir = envDir
	}

	// Check the kind
	kind, _ := rawRequest["kind"].(string)
	
	switch kind {
	case "InfraForge":
		// Parse as InfraForge request
		var request pipeline.InfraForgeRequest
		if err := yaml.Unmarshal(data, &request); err != nil {
			log.Fatalf("Failed to parse InfraForge request: %v", err)
		}
		
		log.Printf("Processing InfraForge request for tenant: %s, environment: %s", 
			request.Spec.Tenant, request.Spec.Environment)
		
		// Create InfraForge processor
		processor := pipeline.NewInfraForgeProcessor(&request, outputDir)
		
		// Process the request
		if err := processor.Process(); err != nil {
			log.Fatalf("Failed to process InfraForge request: %v", err)
		}
		
		// Create metadata for Kratix
		createMetadata(outputDir, request.Spec.Tenant, request.Spec.Environment)
		
	// Legacy PaaSPlatform removed - only InfraForge supported
		
	default:
		log.Fatalf("Unknown request kind: %s", kind)
	}

	log.Println("InfraForge pipeline completed successfully!")
}

func createMetadata(outputDir, tenant, environment string) {
	metadataPath := filepath.Join(outputDir, "metadata.yaml")
	metadata := map[string]interface{}{
		"name": fmt.Sprintf("%s-%s", tenant, environment),
		"labels": map[string]string{
			"tenant":      tenant,
			"environment": environment,
			"managed-by":  "infraforge",
		},
	}

	metadataData, _ := yaml.Marshal(metadata)
	if err := os.WriteFile(metadataPath, metadataData, 0644); err != nil {
		log.Printf("Warning: failed to write metadata: %v", err)
	}
}