package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestChatViewInitialization verifies that chat view is properly initialized
func TestChatViewInitialization(t *testing.T) {
	// Create test model
	model := NewModel()

	// Check if model has initial data
	assert.NotEmpty(t, model.ChatHistory, "Chat history should not be empty")
	assert.Contains(t, model.ChatHistory[0], "Welcome to Hasher CLI!", "Initial welcome message missing")

	// Check if chat view contains initial content
	assert.Contains(t, model.ChatView.View(), "Welcome to Hasher CLI!", "Chat view should display initial content")
}

// TestChatUpdate directly tests the chat update functionality
func TestChatUpdate(t *testing.T) {
	// Create test model
	model := NewModel()

	// Test adding a message directly
	testMessage := "Hello, world!"
	model.ChatHistory = append(model.ChatHistory, userMessageStyle.Render("You: "+testMessage))
	model.updateChatView()

	// Verify the message is in the chat
	assert.Contains(t, model.ChatView.View(), testMessage, "Chat view should contain the new message")
}

// TestLLMResponse verifies that LLM responses are properly added to the chat
func TestLLMResponse(t *testing.T) {
	// Create test model
	model := NewModel()

	// Test adding an LLM response
	testResponse := "This is a test response from LLM!"
	model.ChatHistory = append(model.ChatHistory, llmMessageStyle.Render("Tiny-LLM: "+testResponse))
	model.updateChatView()

	// Verify the response is in the chat
	assert.Contains(t, model.ChatView.View(), testResponse, "Chat view should contain the LLM response")
}
