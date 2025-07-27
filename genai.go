package main

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/vertexai/genai"
)

func (cfg *config) generateResponse(ctx context.Context, session *ChatSession) (string, error) {
	client, err := genai.NewClient(ctx, cfg.clientConfig.Project, cfg.clientConfig.Location)
	if err != nil {
		log.Printf("Failed to create genai client: %v", err)
		return "", fmt.Errorf("failed to create AI client")
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.0-flash-001")
	model.SystemInstruction = &genai.Content{
		Parts: getSystemInstructions(),
	}

	cs := model.StartChat()
	cs.History = session.ChatHistory

	lastMessage := session.ChatHistory[len(session.ChatHistory)-1]
	resp, err := cs.SendMessage(ctx, lastMessage.Parts...)
	if err != nil {
		log.Printf("Error sending message to AI for session %s: %v", session.ID, err)
		return "", fmt.Errorf("error getting response from AI")
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("received an empty response from the AI")
	}

	fullResponse, ok := resp.Candidates[0].Content.Parts[0].(genai.Text)
	if !ok {
		return "", fmt.Errorf("unexpected response format from the AI")
	}

	return string(fullResponse), nil
}

func buildInitialPrompt(articleLink string, timeLimit int) []*genai.Content {
	initialUserPrompt := fmt.Sprintf(
		"Article to analyze: %s\nTime limit for this interview is: %d seconds. Please analyse the article and ask questions about it. If the article answers/solves a problem, the ask the problem statement. If not, just ask general questions",
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
		genai.Text("note that if there is no specific problem answered by the article, just ask general questions"),
		genai.Text("Try to avoid responding with complex jargon. The users are trying to learn, introduce them gradually to the technical jargon"),
		genai.Text("Your role is to guide the user to solve the problem, not provide direct answers. Act as a helpful but challenging system design interviewer."),
		genai.Text("Approach the problem step-by-step, moving from simpler to more complex situations. For example, you might first ask them to design an app, then ask them to scale it for a large number of users."),
		genai.Text("If the user proposes a solution, ask probing questions about trade-offs, scalability, consistency, fault tolerance, availability, and data partitioning."),
		genai.Text("If the user gets stuck, offer a subtle hint or rephrase the question to nudge them toward the right concepts, referencing general system design principles."),
		genai.Text("Identify potential flaws or missing considerations in their proposed solutions and ask them to elaborate on how they would address these."),
		genai.Text("Keep the conversation strictly focused on the problem identified from the blog; do not deviate."),
		genai.Text("Structure your responses as a friendly teacher would."),
		genai.Text("The user's remaining time will be provided with each prompt. If time is low, try to wrap up the discussion. Do not inform the user about the remaining time; only conclude with a polite note and well wishes for their goals once the time is over."),
	}
}
