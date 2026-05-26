// Package shared provides common output helpers for nmcd example programs.
package shared

import (
	"fmt"

	"github.com/opd-ai/nmcd/client"
)

// PrintTxResult prints the common fields of a TxResult.
// The header and pendingNote arguments customise the messages specific to each
// name operation (register vs. update).
func PrintTxResult(result *client.TxResult, header, pendingNote string) {
	fmt.Printf("\n%s\n", header)
	fmt.Println("\nTransaction Result:")
	fmt.Printf("  TX Hash:  %s\n", result.TxHash)
	fmt.Printf("  Name:     %s\n", result.Name)
	fmt.Printf("  Status:   %s\n", result.Status)

	if result.Status == client.TxStatusPending {
		fmt.Printf("\n%s\n", pendingNote)
	} else if result.Status == client.TxStatusConfirmed {
		fmt.Printf("  Block:    %d\n", result.BlockHeight)
		fmt.Printf("  Confirms: %d\n", result.Confirmations)
	}
}
