// SPDX-License-Identifier: GPL-3.0-or-later
// Package brotherql prepares images and encodes the Brother QL raster protocol.
// It is a Go port of Philipp Klaus's brother_ql, with stricter validation and
// corrected multi-page behavior. It operates without printer drivers or CGO;
// hardware communication is provided separately by package printer.
package brotherql
