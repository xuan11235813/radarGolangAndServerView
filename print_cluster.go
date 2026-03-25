package main

import (
	"fmt"
	"strings"
	"time"
)

// printCluster1General prints Cluster1General data in a clear, aligned format
func printCluster1General(msg Cluster1General) {
	timestamp := time.Now().Format("15:04:05.000")

	// Create a clean, aligned output
	fmt.Println("\n" + strings.Repeat("═", 80))
	fmt.Printf(" CLUSTER 1 GENERAL DATA [ID: %d] - %s\n", msg.ID, timestamp)
	fmt.Println(strings.Repeat("─", 80))

	// Print data in two columns for better readability
	fmt.Printf(" %-15s: %12.3f m        %-15s: %12.3f m/s\n",
		"Longitudinal Distance", msg.DistLong,
		"Longitudinal Velocity", msg.VrelLong)

	fmt.Printf(" %-15s: %12.3f m        %-15s: %12.3f m/s\n",
		"Lateral Distance", msg.DistLat,
		"Lateral Velocity", msg.VrelLat)

	fmt.Printf(" %-15s: %12d           %-15s: %12.3f dBsm\n",
		"Dynamic Property", msg.DynProp,
		"RCS", msg.RCS)

	fmt.Println(strings.Repeat("═", 80))
}

// printCluster1GeneralTable prints Cluster1General in a tabular format
func printCluster1GeneralTable(msg Cluster1General) {
	// Simple table format
	fmt.Printf("\n┌────────────────────────────────────────────────────────────┐\n")
	fmt.Printf("│                    CLUSTER 1 GENERAL                      │\n")
	fmt.Printf("├────────────────────────┬──────────────────────────────────┤\n")
	fmt.Printf("│ %-22s │ %-32d │\n", "Object ID", msg.ID)
	fmt.Printf("├────────────────────────┼──────────────────────────────────┤\n")
	fmt.Printf("│ %-22s │ %-32.3f m │\n", "Longitudinal Distance", msg.DistLong)
	fmt.Printf("│ %-22s │ %-32.3f m │\n", "Lateral Distance", msg.DistLat)
	fmt.Printf("│ %-22s │ %-32.3f m/s │\n", "Longitudinal Velocity", msg.VrelLong)
	fmt.Printf("│ %-22s │ %-32.3f m/s │\n", "Lateral Velocity", msg.VrelLat)
	fmt.Printf("│ %-22s │ %-32d │\n", "Dynamic Property", msg.DynProp)
	fmt.Printf("│ %-22s │ %-32.3f dBsm │\n", "Radar Cross Section", msg.RCS)
	fmt.Printf("└────────────────────────┴──────────────────────────────────┘\n")
}

// printCluster1GeneralCompact prints Cluster1General in a compact single line
func printCluster1GeneralCompact(msg Cluster1General) {
	// Compact format for high-frequency updates
	fmt.Printf("[C1G ID:%2d] Dist:{%6.1f,%6.1f}m Vel:{%6.1f,%6.1f}m/s Dyn:%d RCS:%6.1fdBsm\n",
		msg.ID, msg.DistLong, msg.DistLat, msg.VrelLong, msg.VrelLat, msg.DynProp, msg.RCS)
}

// printCluster1GeneralCSV prints Cluster1General in CSV format
func printCluster1GeneralCSV(msg Cluster1General) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Printf("%s,%d,%.3f,%.3f,%.3f,%.3f,%d,%.3f\n",
		timestamp, msg.ID, msg.DistLong, msg.DistLat,
		msg.VrelLong, msg.VrelLat, msg.DynProp, msg.RCS)
}

// printClusterHeader prints a header for the data (call once at start)
func printClusterHeader() {
	fmt.Println("\n" + strings.Repeat("═", 100))
	fmt.Println(" CLUSTER 1 GENERAL DATA - REAL-TIME MONITOR")
	fmt.Println(strings.Repeat("═", 100))
	fmt.Printf("%-6s %-8s %-12s %-12s %-12s %-12s %-8s %-12s\n",
		"TIME", "ID", "DIST_LONG", "DIST_LAT", "VREL_LONG", "VREL_LAT", "DYNPROP", "RCS")
	fmt.Printf("%-6s %-8s %-12s %-12s %-12s %-12s %-8s %-12s\n",
		"", "", "(m)", "(m)", "(m/s)", "(m/s)", "", "(dBsm)")
	fmt.Println(strings.Repeat("─", 100))
}

// printCluster1GeneralLine prints a single line in aligned columns
func printCluster1GeneralLine(msg Cluster1General) {
	timestamp := time.Now().Format("15:04:05")
	fmt.Printf("%-6s %-8d %-12.3f %-12.3f %-12.3f %-12.3f %-8d %-12.3f\n",
		timestamp, msg.ID, msg.DistLong, msg.DistLat,
		msg.VrelLong, msg.VrelLat, msg.DynProp, msg.RCS)
}
