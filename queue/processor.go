package queue

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-smtp/config"
	"go-smtp/delivery"
	"go-smtp/dkim" // Added for DKIM signing
)

// const (
// 	defaultQueueScanInterval = 30 * time.Second // Now from config
// 	defaultMaxAttempts       = 5                // Now from config
// 	defaultRetryInterval     = 5 * time.Minute    // Now from config
// )
const ( // Keep directory names as constants for now, or move to config if desired later
	corruptDirName = "corrupt"
	failedDirName  = "failed"
)

// StartProcessor launches the queue processing goroutine.
func StartProcessor(cfg *config.Config) {
	// Log message already good, uses configured values:
	// log.Printf("INFO: Queue processor configured with ScanInterval: %s, RetryInterval: %s, MaxAttempts: %d",
	// cfg.QueueScanIntervalDuration, cfg.DefaultRetryIntervalDuration, cfg.MaxDeliveryAttempts)
	// The above log is already present from the previous step. Let's ensure it's exactly as required or adjust.
	// The previous step added:
	// log.Printf("INFO: Queue processor configured with ScanInterval: %s, RetryInterval: %s, MaxAttempts: %d",
	//    cfg.QueueScanIntervalDuration, cfg.DefaultRetryIntervalDuration, cfg.MaxDeliveryAttempts)
	// This is good. Let's add a simple "Starting..." message before it.
	log.Println("INFO: Queue processor starting...")
	log.Printf("INFO: Configuration - ScanInterval: %s, RetryInterval: %s, MaxAttempts: %d",
		cfg.QueueScanIntervalDuration, cfg.DefaultRetryIntervalDuration, cfg.MaxDeliveryAttempts)

	// Ensure essential directories exist or can be created
	if err := os.MkdirAll(cfg.MailDir, 0750); err != nil {
		log.Fatalf("FATAL: MailDir %s cannot be created/accessed: %v", cfg.MailDir, err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.MailDir, corruptDirName), 0750); err != nil {
		log.Fatalf("FATAL: Corrupt directory in MailDir %s cannot be created/accessed: %v", cfg.MailDir, err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.MailDir, failedDirName), 0750); err != nil {
		log.Fatalf("FATAL: Failed directory in MailDir %s cannot be created/accessed: %v", cfg.MailDir, err)
	}


	log.Printf("INFO: Queue processor configured with ScanInterval: %s, RetryInterval: %s, MaxAttempts: %d",
		cfg.QueueScanIntervalDuration, cfg.DefaultRetryIntervalDuration, cfg.MaxDeliveryAttempts)

	ticker := time.NewTicker(cfg.QueueScanIntervalDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Println("INFO: Queue processor tick: scanning for emails...")
			processQueueDirectory(cfg)
		}
	}
}

// processQueueDirectory scans the MailDir for .meta files and processes them.
func processQueueDirectory(cfg *config.Config) {
	metaFilePattern := filepath.Join(cfg.MailDir, "*.meta")
	metaFiles, err := filepath.Glob(metaFilePattern)
	if err != nil {
		log.Printf("ERROR: Queue scan: Failed to glob for meta files in %s: %v", cfg.MailDir, err)
		return
	}

	if len(metaFiles) == 0 {
		log.Println("INFO: Queue scan: No messages in queue to process.")
		return
	}
	log.Printf("INFO: Queue scan: Found %d message(s) in queue.", len(metaFiles))

	for _, metaFilePath := range metaFiles {
		log.Printf("INFO: Processing queue file: %s", metaFilePath)

		metadata, err := LoadMetadata(metaFilePath)
		if err != nil {
			log.Printf("ERROR: Failed to load metadata from %s. Error: %v. Moving to '%s' directory.", metaFilePath, err, corruptDirName)
			moveFileToSubdir(cfg.MailDir, corruptDirName, filepath.Base(metaFilePath), fmt.Sprintf("metadata load error: %v", err))
			emlFileName := strings.TrimSuffix(filepath.Base(metaFilePath), ".meta") + ".eml"
			emlPath := filepath.Join(cfg.MailDir, emlFileName)
			if _, statErr := os.Stat(emlPath); statErr == nil {
				moveFileToSubdir(cfg.MailDir, corruptDirName, emlFileName, "associated with corrupt metadata")
			}
			continue
		}

		if time.Now().Before(metadata.NextAttemptTime) {
			log.Printf("INFO: Skipping MessageID: %s, next attempt not due until %s", metadata.MessageID, metadata.NextAttemptTime.Format(time.RFC3339))
			continue
		}

		emlFilePath := strings.TrimSuffix(metaFilePath, ".meta") + ".eml"
		emailData, err := os.ReadFile(emlFilePath)
		if err != nil {
			log.Printf("ERROR: Missing .eml file %s for metadata %s. Error: %v. Moving metadata to '%s' directory.", emlFilePath, metaFilePath, err, corruptDirName)
			moveFileToSubdir(cfg.MailDir, corruptDirName, filepath.Base(metaFilePath), fmt.Sprintf("missing .eml file, error: %v", err))
			continue
		}

		log.Printf("INFO: Attempting delivery for MessageID: %s (Attempt %d/%d) to recipients: %v",
			metadata.MessageID, metadata.AttemptCount+1, cfg.MaxDeliveryAttempts, metadata.Recipients)
		emailToSend := delivery.Email{
			From:     metadata.Sender,
			To:       metadata.Recipients,
			Data:     emailData, // Original email data
			Hostname: cfg.ServerHostname,
		}

		// Attempt DKIM signing if enabled and configured
		currentEmailData := emailData // Start with original data
		if cfg.DKIMEnable {
			if cfg.DKIMDomain != "" && cfg.DKIMSelector != "" && cfg.DKIMPrivateKeyPath != "" {
				log.Printf("INFO: MessageID: %s attempting DKIM signing. Domain: %s, Selector: %s", metadata.MessageID, cfg.DKIMDomain, cfg.DKIMSelector)

				dkimOpts := dkim.DKIMSignOptions{
					Domain:         cfg.DKIMDomain,
					Selector:       cfg.DKIMSelector,
					PrivateKeyPath: cfg.DKIMPrivateKeyPath,
					HeadersToSign:  cfg.DKIMHeaders,
				}

				signedEmailData, signErr := dkim.SignEmail(currentEmailData, dkimOpts) // Pass a copy if SignEmail modifies in place and you need original
				if signErr != nil {
					log.Printf("ERROR: MessageID: %s failed DKIM signing. Error: %v. Sending email unsigned.", metadata.MessageID, signErr)
					// emailToSend.Data remains originalEmailData
				} else {
					log.Printf("INFO: MessageID: %s successfully signed with DKIM.", metadata.MessageID)
					emailToSend.Data = signedEmailData // Use the signed email data
				}
			} else {
				log.Printf("WARN: MessageID: %s DKIM signing enabled but not all required DKIM configurations (Domain, Selector, PrivateKeyPath) are set. Sending unsigned.", metadata.MessageID)
			}
		}
		// emailToSend.Data now contains either the original or DKIM-signed data

		err = delivery.SendEmail(&emailToSend, cfg) // Pass config for outbound policies
		metadata.LastAttemptTime = time.Now()

		if err == nil {
			log.Printf("INFO: Email MessageID: %s delivered successfully to recipients: %v. Removing from queue.", metadata.MessageID, metadata.Recipients)
			if errRem := os.Remove(metaFilePath); errRem != nil {
				log.Printf("ERROR: Delivered MessageID: %s, but FAILED to remove metadata file %s: %v", metadata.MessageID, metaFilePath, errRem)
			} else {
				log.Printf("INFO: Removed metadata file %s for delivered MessageID: %s.", metaFilePath, metadata.MessageID)
			}
			if errRem := os.Remove(emlFilePath); errRem != nil {
				log.Printf("ERROR: Delivered MessageID: %s, but FAILED to remove email file %s: %v", metadata.MessageID, emlFilePath, errRem)
			} else {
				log.Printf("INFO: Removed email file %s for delivered MessageID: %s.", emlFilePath, metadata.MessageID)
			}
		} else {
			currentAttempt := metadata.AttemptCount + 1 // This is the attempt that just failed
			metadata.AttemptCount = currentAttempt     // Update before saving
			metadata.LastError = err.Error()

			if metadata.AttemptCount >= cfg.MaxDeliveryAttempts {
				log.Printf("ERROR: Email MessageID: %s failed permanently after %d/%d attempts. Error: %s. Moving to '%s' directory.",
					metadata.MessageID, metadata.AttemptCount, cfg.MaxDeliveryAttempts, err.Error(), failedDirName)
				handlePermanentFailure(cfg.MailDir, metadata, emlFilePath, metaFilePath, err.Error())
			} else {
				metadata.NextAttemptTime = time.Now().Add(cfg.DefaultRetryIntervalDuration)
				if errSave := SaveMetadata(metaFilePath, metadata); errSave != nil {
					log.Printf("ERROR: Failed to save updated metadata for MessageID: %s after temporary failure (Attempt %d/%d). Error: %v. Email will be re-processed with old state on next scan.",
						metadata.MessageID, metadata.AttemptCount, cfg.MaxDeliveryAttempts, errSave)
				} else {
					log.Printf("WARN: Temporary delivery failure for MessageID: %s (Attempt %d/%d) to %v. Error: %s. Next attempt at: %s. Updated metadata saved.",
						metadata.MessageID, metadata.AttemptCount, cfg.MaxDeliveryAttempts, metadata.Recipients, err.Error(), metadata.NextAttemptTime.Format(time.RFC3339))
				}
			}
		}
	}
}

// handlePermanentFailure moves the .eml and .meta files to the "failed" subdirectory.
func handlePermanentFailure(mailDir string, metadata *MailMetadata, emlFilePath string, metaFilePath string, errMsg string) {
	finalErrMsg := fmt.Sprintf("PERMANENT FAILURE (attempts: %d): %s", metadata.AttemptCount, errMsg)
	log.Printf("ERROR: Email MessageID: %s processing for permanent failure. Final Error: %s", metadata.MessageID, finalErrMsg)

	metadata.LastError = finalErrMsg
	metadata.NextAttemptTime = time.Time{} // Mark as no more attempts
	if err := SaveMetadata(metaFilePath, metadata); err != nil {
		log.Printf("WARN: Could not update metadata for MessageID: %s with final error before moving to '%s': %v", metadata.MessageID, failedDirName, err)
		// Proceed with moving the files anyway
	}

	moveFileToSubdir(mailDir, failedDirName, filepath.Base(metaFilePath), "permanent delivery failure")
	moveFileToSubdir(mailDir, failedDirName, filepath.Base(emlFilePath), "permanent delivery failure")
}

// moveFileToSubdir attempts to move a file from the mailDir root to a specified subdirectory within mailDir.
func moveFileToSubdir(mailDir, subDirName, fileName, reason string) {
	sourcePath := filepath.Join(mailDir, fileName)
	destDirPath := filepath.Join(mailDir, subDirName)

	// Check if source file exists before attempting to move
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		log.Printf("WARN: Source file %s does not exist, cannot move. (Reason: %s)", sourcePath, reason)
		return
	}

	// Ensure the destination subdirectory exists
	if err := os.MkdirAll(destDirPath, 0750); err != nil {
		log.Printf("ERROR: Could not create/access directory %s to move file %s: %v", destDirPath, fileName, err)
		log.Printf("ERROR: File %s (reason: %s) remains in mail root directory %s due to move error.", fileName, reason, mailDir)
		return
	}

	destPath := filepath.Join(destDirPath, fileName)
	log.Printf("INFO: Moving %s to %s (Reason: %s)", sourcePath, destPath, reason)
	if err := os.Rename(sourcePath, destPath); err != nil {
		log.Printf("ERROR: Failed to move %s to %s: %v. File may remain in original location.", sourcePath, destPath, err)
	}
}
