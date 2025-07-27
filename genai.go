package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"cloud.google.com/go/vertexai/genai"
	"google.golang.org/api/option"
)

func (cfg *config) generateAndStreamResponse(w http.ResponseWriter, r *http.Request, session *ChatSession) {
	ctx := r.Context()
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Println("ResponseWriter does not support flushing")
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	client, err := genai.NewClient(ctx, cfg.clientConfig.Project, cfg.clientConfig.Location, option.WithAPIKey(cfg.apiKey))
	if err != nil {
		log.Printf("Failed to create genai client: %v", err)
		return
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.0-flash-001")
	model.SystemInstruction = &genai.Content{
		Parts: getSystemInstructions(),
	}

	cs := model.StartChat()
	cs.History = session.ChatHistory

	iter := cs.SendMessageStream(ctx, genai.Text("continue"))

	var llmResponseBuilder strings.Builder
	for {
		resp, err := iter.Next()
		if err == io.EOF {
			log.Printf("Stream finished for session %s", session.ID)
			break
		}
		if err != nil {
			log.Printf("Error receiving chunk from AI for session %s: %v", session.ID, err)
			return
		}

		if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
			chunk, ok := resp.Candidates[0].Content.Parts[0].(genai.Text)
			if ok {
				// write chunks to the response here,
				fmt.Fprintf(w, "data: %s\n\n", string(chunk))
				flusher.Flush()
				// this is for the storing the full thing to add as the context
				llmResponseBuilder.WriteString(string(chunk))
			}
		}
	}

	fullResponse := llmResponseBuilder.String()
	if fullResponse != "" {
		session.ChatHistory = append(session.ChatHistory, &genai.Content{
			Parts: []genai.Part{genai.Text(fullResponse)},
			Role:  "model",
		})
	}
	// config := &genai.GenerateContentConfig{
	// 	SystemInstruction: &genai.Content{
	// 		Parts: []*genai.Part{
	// 			{Text: "Analyze the provided blog content to identify the primary system design problem and formulate it as a concise interview question (e.g., 'Design a system for X...'). Keep the question strictly focused on the problem described in the text."},
	// 			{Text: "Your role is to guide the user to solve the problem, not provide direct answers. Act as a helpful but challenging system design interviewer."},
	// 			{Text: "Approach the problem step-by-step, moving from simpler to more complex situations. For example, you might first ask them to design an app, then ask them to scale it for a large number of users."},
	// 			{Text: "If the user proposes a solution, ask probing questions about trade-offs, scalability, consistency, fault tolerance, availability, and data partitioning."},
	// 			{Text: "If the user gets stuck, offer a subtle hint or rephrase the question to nudge them toward the right concepts, referencing general system design principles."},
	// 			{Text: "Identify potential flaws or missing considerations in their proposed solutions and ask them to elaborate on how they would address these."},
	// 			{Text: "Keep the conversation strictly focused on the problem identified from the blog; do not deviate."},
	// 			{Text: "Structure your responses as an friendly teacher would."},
	// 			{Text: "The user's remaining time will be provided with each response. If time is low, try to wrap up the discussion. Do not inform the user about the remaining time; only conclude with a polite note and well wishes for their goals once the time is over."},
	// 			{Text: "the new input will contain: {userReply, timeRemaining}"},
	// 		},
	// 	},
	// }

	// for resp, err := range client.Models.GenerateContentStream(ctx, modelName, contents, config) {
	// 	if err != nil {
	// 		return
	// 	}

	// 	chunk := resp.Text()

	// 	fmt.Fprintln(w, chunk)
	// }
}

func buildInitialPrompt(articleLink string, timeLimit int) []*genai.Content {
	initialUserPrompt := fmt.Sprintf(
		"Article to analyze: %s\nTime limit for this interview is: %d seconds. Please begin.",
		articleLink, timeLimit,
	)
	return []*genai.Content{
		{
			Parts: []genai.Part{genai.Text(initialUserPrompt)},
			Role:  "user",
		},
	}
}

func getSystemInstructions() []genai.Part {
	return []genai.Part{
		genai.Text("Analyze the provided blog content to identify the primary system design problem and formulate it as a concise interview question (e.g., 'Design a system for X...'). Keep the question strictly focused on the problem described in the text."),
		genai.Text("Your role is to guide the user to solve the problem, not provide direct answers. Act as a helpful but challenging system design interviewer."),
		genai.Text("Approach the problem step-by-step, moving from simpler to more complex situations. For example, you might first ask them to design an app, then ask them to scale it for a large number of users."),
		genai.Text("If the user proposes a solution, ask probing questions about trade-offs, scalability, consistency, fault tolerance, availability, and data partitioning."),
		genai.Text("If the user gets stuck, offer a subtle hint or rephrase the question to nudge them toward the right concepts, referencing general system design principles."),
		genai.Text("Identify potential flaws or missing considerations in their proposed solutions and ask them to elaborate on how they would address these."),
		genai.Text("Keep the conversation strictly focused on the problem identified from the blog; do not deviate."),
		genai.Text("Structure your responses as a friendly teacher would."),
		genai.Text("The user's remaining time will be provided with each response. If time is low, try to wrap up the discussion. Do not inform the user about the remaining time; only conclude with a polite note and well wishes for their goals once the time is over."),
		genai.Text("The new input will contain: {userReply, timeRemaining}"),
	}
}
