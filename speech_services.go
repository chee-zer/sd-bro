package main

import (
	"context"
	"fmt"
	"log"

	speech "cloud.google.com/go/speech/apiv1"
	"cloud.google.com/go/speech/apiv1/speechpb"
	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
)

func (cfg *config) convertSpeechToText(ctx context.Context, audioData []byte) (string, error) {
	client, err := speech.NewClient(ctx)
	if err != nil {
		log.Println("Falied to create Speech-To-Text client: ", err)
		return "", fmt.Errorf("failed to create new speect client")
	}
	defer client.Close()

	resp, err := client.Recognize(ctx, &speechpb.RecognizeRequest{
		Config: &speechpb.RecognitionConfig{
			Encoding:     speechpb.RecognitionConfig_WEBM_OPUS,
			LanguageCode: "en-US",
			Model:        "telephony",
		},
		Audio: &speechpb.RecognitionAudio{
			AudioSource: &speechpb.RecognitionAudio_Content{Content: audioData},
		},
	})
	if err != nil {
		log.Println("Failed to recognize speech: ", err)
		return "", fmt.Errorf("failed to recognize speech")
	}

	if len(resp.Results) > 0 && len(resp.Results[0].Alternatives) > 0 {
		return resp.Results[0].Alternatives[0].Transcript, nil
	}

	return "", fmt.Errorf("no transcript found")
}

func (cfg *config) convertTextToSpeech(ctx context.Context, text string) ([]byte, error) {
	client, err := texttospeech.NewClient(ctx)
	if err != nil {
		log.Printf("Failed to create Text-to-Speech client: %v", err)
		return nil, fmt.Errorf("failed to create tts client")
	}
	defer client.Close()

	resp, err := client.SynthesizeSpeech(ctx, &texttospeechpb.SynthesizeSpeechRequest{
		Input: &texttospeechpb.SynthesisInput{
			InputSource: &texttospeechpb.SynthesisInput_Text{Text: text},
		},
		Voice: &texttospeechpb.VoiceSelectionParams{
			LanguageCode: "en-US",
			Name:         "en-US-Wavenet-F",
		},
		AudioConfig: &texttospeechpb.AudioConfig{
			AudioEncoding: texttospeechpb.AudioEncoding_MP3,
		},
	})
	if err != nil {
		log.Printf("Failed to synthesize speech: %v", err)
		return nil, fmt.Errorf("failed to synthesize speech")
	}

	return resp.AudioContent, nil
}
