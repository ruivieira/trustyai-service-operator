package lmes

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	lmesv1alpha1 "github.com/trustyai-explainability/trustyai-service-operator/api/lmes/v1alpha1"
)

// ValidateJSON checks that the input is a valid standalone JSON
func ValidateJSON(input string) error {
	decoder := json.NewDecoder(strings.NewReader(input))

	var data interface{}
	if err := decoder.Decode(&data); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if decoder.More() {
		return fmt.Errorf("extra data found after valid JSON")
	}

	return nil
}

// ValidateJSONContent validates that JSON content is safe for execution
func ValidateJSONContent(input string) error {
	// First validate that it's proper JSON
	if err := ValidateJSON(input); err != nil {
		return err
	}

	commandInjectionPatterns := []string{
		"$(", "`", "&&", "||",
	}

	for _, pattern := range commandInjectionPatterns {
		if strings.Contains(input, pattern) {
			return fmt.Errorf("JSON content contains dangerous pattern: %s", pattern)
		}
	}

	return nil
}

// ValidateCustomArtifactValue validates CustomArtifact.Value which can be either JSON or plain text
func ValidateCustomArtifactValue(input string) error {
	// Try to parse as JSON first
	if err := ValidateJSON(input); err == nil {
		// It's valid JSON, apply JSON-specific validation
		return ValidateJSONContent(input)
	}

	// It's not JSON, treat as plain text and validate for shell metacharacters
	if ContainsShellMetacharacters(input) {
		return fmt.Errorf("custom artifact value contains shell metacharacters")
	}

	// Additional validation for plain text content that could be invalid
	dangerousPatterns := []string{
		"$(", "`", "&&", "||", ";",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(input, pattern) {
			return fmt.Errorf("custom artifact value contains dangerous pattern: %s", pattern)
		}
	}

	return nil
}

// ValidateUserInput validates all user fields
func ValidateUserInput(job *lmesv1alpha1.LMEvalJob) error {
	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	// Validate model name
	if err := ValidateModelName(job.Spec.Model); err != nil {
		return fmt.Errorf("invalid model: %w", err)
	}

	// Validate model arguments
	if err := ValidateArgs(job.Spec.ModelArgs, "model argument"); err != nil {
		return fmt.Errorf("invalid model arguments: %w", err)
	}

	// Validate task names
	if err := ValidateTaskNames(job.Spec.TaskList.TaskNames); err != nil {
		return fmt.Errorf("invalid task names: %w", err)
	}

	// Validate task recipes
	if err := ValidateTaskRecipes(job.Spec.TaskList.TaskRecipes); err != nil {
		return fmt.Errorf("invalid task recipes: %w", err)
	}

	// Validate generation arguments (genArgs)
	if err := ValidateArgs(job.Spec.GenArgs, "generation argument"); err != nil {
		return fmt.Errorf("invalid generation arguments: %w", err)
	}

	// Validate limit field
	if job.Spec.Limit != "" {
		if err := ValidateLimit(job.Spec.Limit); err != nil {
			return fmt.Errorf("invalid limit: %w", err)
		}
	}

	// Validate batch size
	if job.Spec.BatchSize != nil {
		if err := ValidateBatchSizeInput(*job.Spec.BatchSize); err != nil {
			return fmt.Errorf("invalid batch size: %w", err)
		}
	}

	return nil
}

// ValidateModelName validates model name
func ValidateModelName(model string) error {
	if model == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	if _, ok := AllowedModels[model]; !ok {
		return fmt.Errorf("model name must be one of: hf, openai-completions, openai-chat-completions, local-completions, local-chat-completions, watsonx_llm, textsynth")
	}

	return nil
}

// ValidateArgs validates argument arrays
func ValidateArgs(args []lmesv1alpha1.Arg, argType string) error {
	for i, arg := range args {
		if err := ValidateArgName(arg.Name); err != nil {
			return fmt.Errorf("%s[%d] name: %w", argType, i, err)
		}
		if err := ValidateArgValue(arg.Value); err != nil {
			return fmt.Errorf("%s[%d] value: %w", argType, i, err)
		}
	}
	return nil
}

// ValidateTaskRecipes validates task recipe arrays
func ValidateTaskRecipes(recipes []lmesv1alpha1.TaskRecipe) error {
	for i, recipe := range recipes {
		// Validate card
		if recipe.Card.Name != "" {
			if err := ValidateTaskName(recipe.Card.Name); err != nil {
				return fmt.Errorf("recipe[%d] card name: %w", i, err)
			}
		}
		if recipe.Card.Custom != "" {
			if err := ValidateJSONContent(recipe.Card.Custom); err != nil {
				return fmt.Errorf("recipe[%d] custom card: %w", i, err)
			}
		}

		// Validate template
		if recipe.Template != "" {
			if err := ValidateTaskName(recipe.Template); err != nil {
				return fmt.Errorf("recipe[%d] template: %w", i, err)
			}
		}

		// Validate task
		if recipe.Task != nil {
			if err := ValidateTaskName(*recipe.Task); err != nil {
				return fmt.Errorf("recipe[%d] task: %w", i, err)
			}
		}

		// Validate metrics
		for j, metric := range recipe.Metrics {
			if err := ValidateTaskName(metric); err != nil {
				return fmt.Errorf("recipe[%d] metric[%d]: %w", i, j, err)
			}
		}

		// Validate format
		if recipe.Format != nil {
			if err := ValidateTaskName(*recipe.Format); err != nil {
				return fmt.Errorf("recipe[%d] format: %w", i, err)
			}
		}
	}
	return nil
}

// ValidateArgName validates argument names
func ValidateArgName(name string) error {
	if name == "" {
		return fmt.Errorf("argument name cannot be empty")
	}

	if !PatternArgName.MatchString(name) {
		return fmt.Errorf("argument name contains invalid characters (only alphanumeric, ., _, - allowed)")
	}

	return nil
}

// ValidateArgValue validates argument values
func ValidateArgValue(value string) error {
	if !PatternArgValue.MatchString(value) {
		return fmt.Errorf("argument value contains invalid characters (only alphanumeric, ., _, /, :, -, and spaces allowed)")
	}

	// Additional check for shell metacharacters
	if ContainsShellMetacharacters(value) {
		return fmt.Errorf("argument value contains shell metacharacters")
	}

	return nil
}

// ValidateLimit validates limit field format
func ValidateLimit(limit string) error {
	if !PatternLimit.MatchString(limit) {
		return fmt.Errorf("limit must be a valid number")
	}

	// additional check
	if ContainsShellMetacharacters(limit) {
		return fmt.Errorf("limit contains shell metacharacters")
	}

	return nil
}

// ValidateTaskNames validates task name arrays
func ValidateTaskNames(taskNames []string) error {
	for i, taskName := range taskNames {
		if err := ValidateTaskName(taskName); err != nil {
			return fmt.Errorf("task name[%d]: %w", i, err)
		}
	}
	return nil
}

// ValidateTaskName validates individual task names
func ValidateTaskName(name string) error {
	if name == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	if !PatternTaskName.MatchString(name) {
		return fmt.Errorf("task name contains invalid characters (only alphanumeric, ., _, - allowed)")
	}

	return nil
}

// ContainsShellMetacharacters checks for dangerous shell characters
// Allows JSON-specific characters like {}, [] but blocks command execution patterns
func ContainsShellMetacharacters(input string) bool {
	// We exclude {, }, [, ] as they're needed for JSON content
	shellChars := []string{
		";", "&", "|", "$", "`", "(", ")",
		"<", ">", "\"", "'", "\\", "\n", "\r", "\t",
	}

	for _, char := range shellChars {
		if strings.Contains(input, char) {
			return true
		}
	}

	return false
}

// ValidateBatchSizeInput validates batch size input format
func ValidateBatchSizeInput(batchSize string) error {
	if batchSize == "" {
		return fmt.Errorf("batch size cannot be empty")
	}

	// Check for shell metacharacters
	if ContainsShellMetacharacters(batchSize) {
		return fmt.Errorf("batch size contains shell metacharacters")
	}

	// Allow "auto", "auto:N", or plain numbers
	validPatterns := []string{
		`^auto$`,
		`^auto:\d+$`,
		`^\d+$`,
	}

	for _, pattern := range validPatterns {
		if regexp.MustCompile(pattern).MatchString(batchSize) {
			return nil
		}
	}

	return fmt.Errorf("batch size must be 'auto', 'auto:N', or a positive integer")
}
