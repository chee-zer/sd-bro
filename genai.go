package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	genai "google.golang.org/genai"
)

func (cfg *config) generateAndStreamResponse(w http.ResponseWriter, r *http.Request, session *ChatSession, prompt string) {
	ctx := context.Background()

	client, err := genai.NewClient(ctx, cfg.clientConfig)
	if err != nil {
		log.Println("Failed to create genai client: ", err)
		return
	}

	modelName := "gemini-2.0-flash-001"
	contents := genai.Text(prompt)
	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: "Analyze the provided blog content to identify the primary system design problem and formulate it as a concise interview question (e.g., 'Design a system for X...'). Keep the question strictly focused on the problem described in the text."},
				{Text: "Your role is to guide the user to solve the problem, not provide direct answers. Act as a helpful but challenging system design interviewer."},
				{Text: "Approach the problem step-by-step, moving from simpler to more complex situations. For example, you might first ask them to design an app, then ask them to scale it for a large number of users."},
				{Text: "If the user proposes a solution, ask probing questions about trade-offs, scalability, consistency, fault tolerance, availability, and data partitioning."},
				{Text: "If the user gets stuck, offer a subtle hint or rephrase the question to nudge them toward the right concepts, referencing general system design principles."},
				{Text: "Identify potential flaws or missing considerations in their proposed solutions and ask them to elaborate on how they would address these."},
				{Text: "Keep the conversation strictly focused on the problem identified from the blog; do not deviate."},
				{Text: "Structure your responses as an friendly teacher would."},
				{Text: "The user's remaining time will be provided with each response. If time is low, try to wrap up the discussion. Do not inform the user about the remaining time; only conclude with a polite note and well wishes for their goals once the time is over."},
				{Text: "the new input will contain: {userReply, timeRemaining}"},
			},
		},
	}

	for resp, err := range client.Models.GenerateContentStream(ctx, modelName, contents, config) {
		if err != nil {
			return
		}

		chunk := resp.Text()

		fmt.Fprintln(w, chunk)
	}
}
