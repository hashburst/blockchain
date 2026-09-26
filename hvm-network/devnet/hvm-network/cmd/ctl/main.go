package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hashburst/blockchain"
	"hashburst/consensus"
	hvm_network "hashburst/devnet/hvm-network/internal/devnet"
	"hashburst/hvm"
	"hashburst/protocolv2"
	"hashburst/wallet"
)

const (
	validatorUnbondCompute   = uint64(25_000)
	validatorEvidenceCompute = uint64(120_000)
)

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type contractCallPayload struct {
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args"`
}

func main() {
	manifestPath := flag.String("manifest", "", "manifest path")
	outDir := flag.String("out", "", "fixture output directory containing secrets/")
	command := flag.String("command", "canary", "canary|slash|unbond|status")
	target := flag.Int("target", 1, "target node index for slash/unbond")
	flag.Parse()
	if *manifestPath == "" || *outDir == "" {
		fatalf("--manifest and --out are required")
	}
	var manifest hvm_network.Manifest
	must(hvm_network.LoadJSON(*manifestPath, &manifest))
	switch *command {
	case "canary":
		must(runCanary(manifest, *outDir))
	case "slash":
		must(runSlash(manifest, *outDir, *target))
	case "unbond":
		must(runUnbond(manifest, *outDir, *target))
	case "status":
		must(runStatus(manifest))
	default:
		fatalf("unknown --command %q", *command)
	}
}

func runCanary(m hvm_network.Manifest, out string) error {
	recorder, err := loadRecorder(out)
	if err != nil {
		return err
	}
	cfg := blockchain.DefaultProtocolV2Config()
	cfg.ChainID = m.ChainID

	deployLimit := uint64(600_000)
	deployFee, _ := cfg.FeePolicy.MaxFeeForLimit(deployLimit)
	initRaw, _ := json.Marshal(hvm.MiningPayoutRegistryInit{Admin: recorder.Address(), Recorders: []string{recorder.Address()}, ProfileWriters: []string{recorder.Address()}})
	deployData, _ := json.Marshal(hvm.DeployRequest{ContractType: hvm.MiningPayoutRegistryType, Init: initRaw})
	seq, err := querySequence(m, recorder.Address())
	if err != nil {
		return err
	}
	deploy := protocolv2.NewTransactionV2(m.ChainID, protocolv2.TxContractDeploy, recorder.Address(), "", 0, seq, deployLimit, deployFee, deployData)
	if err := deploy.Sign(recorder); err != nil {
		return err
	}
	deployReceipt, err := submitAndWait(m, deploy, 25*time.Second)
	if err != nil {
		return fmt.Errorf("MPR deploy: %w", err)
	}
	if !deployReceipt.Success || deployReceipt.Contract == "" {
		return fmt.Errorf("MPR deploy reverted: %s", deployReceipt.RevertReason)
	}
	contract := deployReceipt.Contract

	// Synthetic private fixture. These source values never enter HVM calldata.
	namespace := randomBytes(32)
	rawAPI := randomBytes(24)
	rawWallet := randomBytes(32)
	userRef := hmacHex(namespace, append([]byte("HASHBURST_USER_REF_V1\x00"), rawAPI...))
	walletMessage := append([]byte(userRef+"\x00DOGE\x00mainnet\x00"), rawWallet...)
	walletRef := hmacHex(namespace, walletMessage)
	apiSalt := hmacBytes(namespace, append([]byte("HASHBURST_APIKEY_SALT_V1\x00"), rawAPI...))
	walletSalt := hmacBytes(namespace, append([]byte("HASHBURST_WALLET_SALT_V1\x00"), walletMessage...))
	apiCommit := commitment("HASHBURST_APIKEY_COMMIT_V1", rawAPI, apiSalt)
	addrCommit := commitment("HASHBURST_ADDRESS_COMMIT_V1", rawWallet, walletSalt)
	sourceCommit := hashHex(append([]byte("HASHBURST_SOURCE_RECORD_V1\x00"), randomBytes(48)...))
	profile := hvm.StakeholderProfileRequest{
		UserRef: userRef, APIKeyCommitment: apiCommit, ProfileVersion: 1,
		SourceRecordCommitment: sourceCommit, UpdatedAt: uint64(time.Now().Unix()),
		Wallets: []hvm.StakeholderWalletCommitment{{WalletRef: walletRef, Coin: "DOGE", Network: "mainnet", AddressScheme: "DOGE_BASE58", AddressCommitment: addrCommit}},
	}
	if _, err := callContract(m, recorder, contract, "registerStakeholderProfile", profile, 700_000); err != nil {
		return fmt.Errorf("stakeholder profile: %w", err)
	}

	payoutSent := hashLabel("hvm-network-private-payout-sent")
	payoutCancelled := hashLabel("hvm-network-private-payout-cancelled")
	baseItem := func(id string, amount string) hvm.PayoutItem {
		return hvm.PayoutItem{
			PayoutID: id, UserRef: userRef, StakeholderProfileVersion: 1,
			WalletRef: walletRef, AddressCommitment: addrCommit, ExternalAddressScheme: "DOGE_BASE58",
			AuditedAmountAtomic: amount, ContributionNumerator: 1000, ContributionDenominator: 1_000_000,
		}
	}
	ts := uint64(time.Now().Unix())
	auditSent := hvm.AuditBatchRequest{
		ReportID: hashLabel("hvm-network-report-sent"), BatchID: hashLabel("hvm-network-batch-sent"), BatchIndex: 0, BatchCount: 1,
		Pool: "ViaBTC", Currency: "DOGE", CurrencyDecimals: 8, SourceTimestamp: ts,
		EvidenceHash: hashLabel("hvm-network-auditing-evidence-sent"), Items: []hvm.PayoutItem{baseItem(payoutSent, "1500000000")},
	}
	if _, err := callContract(m, recorder, contract, "recordAuditBatch", auditSent, 900_000); err != nil {
		return fmt.Errorf("AUDITING/SENT fixture: %w", err)
	}
	extTx := strings.TrimPrefix(hashLabel("hvm-network-public-doge-transfer"), "0x")
	transfer := hvm.ExternalTransfer{
		TransferID: hvm.ExternalTransferID("DOGE", "mainnet", extTx), ExternalChain: "DOGE", Network: "mainnet", TxID: extTx,
		ObservedTimestamp: ts + 1, EvidenceHash: hashLabel("hvm-network-sent-api-evidence"),
	}
	sent := hvm.TransitionBatchRequest{
		Status: hvm.StatusSent, EvidenceHash: transfer.EvidenceHash, ObservedTimestamp: ts + 1,
		ExternalTransfer: &transfer, Items: []hvm.TransitionItem{{PayoutID: payoutSent, ObservedAmountAtomic: "1500000000"}},
	}
	if _, err := callContract(m, recorder, contract, "recordTransitionBatch", sent, 900_000); err != nil {
		return fmt.Errorf("SENT transition: %w", err)
	}

	auditCancelled := hvm.AuditBatchRequest{
		ReportID: hashLabel("hvm-network-report-cancel"), BatchID: hashLabel("hvm-network-batch-cancel"), BatchIndex: 0, BatchCount: 1,
		Pool: "ViaBTC", Currency: "DOGE", CurrencyDecimals: 8, SourceTimestamp: ts + 2,
		EvidenceHash: hashLabel("hvm-network-auditing-evidence-cancel"), Items: []hvm.PayoutItem{baseItem(payoutCancelled, "250000000")},
	}
	if _, err := callContract(m, recorder, contract, "recordAuditBatch", auditCancelled, 900_000); err != nil {
		return fmt.Errorf("AUDITING/CANCELLED fixture: %w", err)
	}
	cancelled := hvm.TransitionBatchRequest{
		Status: hvm.StatusCancelled, EvidenceHash: hashLabel("hvm-network-cancel-evidence"), ObservedTimestamp: ts + 3,
		ReasonCode: "DEVNET_REJECTED", ReasonHash: hashLabel("hvm-network-cancel-reason"),
		Items: []hvm.TransitionItem{{PayoutID: payoutCancelled}},
	}
	if _, err := callContract(m, recorder, contract, "recordTransitionBatch", cancelled, 900_000); err != nil {
		return fmt.Errorf("CANCELLED transition: %w", err)
	}

	curSent, err := getPayout(m, recorder.Address(), contract, payoutSent)
	if err != nil {
		return err
	}
	curCancelled, err := getPayout(m, recorder.Address(), contract, payoutCancelled)
	if err != nil {
		return err
	}
	if curSent.Status != hvm.StatusSent || curCancelled.Status != hvm.StatusCancelled {
		return fmt.Errorf("canary status mismatch: sent=%s cancelled=%s", curSent.Status, curCancelled.Status)
	}
	if curSent.ExternalAddressRaw != "" || curCancelled.ExternalAddressRaw != "" {
		return fmt.Errorf("private canary unexpectedly exposed raw external address")
	}
	fmt.Printf("HVM_NETWORK_MPR_CANARY_OK contract=%s sent=%s cancelled=%s user_ref=%s wallet_ref=%s\n", contract, payoutSent, payoutCancelled, userRef, walletRef)
	return nil
}

func runSlash(m hvm_network.Manifest, out string, target int) error {
	if target < 0 || target >= len(m.Nodes) {
		return fmt.Errorf("target outside manifest")
	}
	recorder, err := loadRecorder(out)
	if err != nil {
		return err
	}
	var sec hvm_network.NodeSecret
	if err := hvm_network.LoadJSON(filepath.Join(out, fmt.Sprintf("node%d", target), "secrets.json"), &sec); err != nil {
		return err
	}
	consKey, err := wallet.FromPrivateKeyHex(sec.ConsensusPrivateHex)
	if err != nil {
		return err
	}
	height, root, err := nextValidatorSet(m)
	if err != nil {
		return err
	}
	a, err := consensus.NewSignedPrevote(m.ChainID, height, 0, hashLabel("hvm-network-equivocation-a"), root, m.Nodes[target].ValidatorID, consKey)
	if err != nil {
		return err
	}
	b, err := consensus.NewSignedPrevote(m.ChainID, height, 0, hashLabel("hvm-network-equivocation-b"), root, m.Nodes[target].ValidatorID, consKey)
	if err != nil {
		return err
	}
	ev := consensus.BFTDoubleSignEvidence{Step: consensus.StepPrevote, Prevote: &consensus.PrevoteEquivocationEvidence{VoteA: a, VoteB: b}}
	data, _ := json.Marshal(ev)
	cfg := blockchain.DefaultProtocolV2Config()
	fee, _ := cfg.FeePolicy.ComputeFee(validatorEvidenceCompute)
	seq, err := querySequence(m, recorder.Address())
	if err != nil {
		return err
	}
	tx := protocolv2.NewTransactionV2(m.ChainID, protocolv2.TxValidatorEvidence, recorder.Address(), "", 0, seq, validatorEvidenceCompute, fee, data)
	if err := tx.Sign(recorder); err != nil {
		return err
	}
	r, err := submitAndWait(m, tx, 25*time.Second)
	if err != nil {
		return err
	}
	if !r.Success {
		return fmt.Errorf("slash receipt reverted: %s", r.RevertReason)
	}
	fmt.Printf("HVM_NETWORK_SLASH_OK target=%d validator=%s tx=0x%s\n", target, m.Nodes[target].ValidatorID, tx.HashHex())
	return nil
}

func runUnbond(m hvm_network.Manifest, out string, target int) error {
	if target < 0 || target >= len(m.Nodes) {
		return fmt.Errorf("target outside manifest")
	}
	var sec hvm_network.NodeSecret
	if err := hvm_network.LoadJSON(filepath.Join(out, fmt.Sprintf("node%d", target), "secrets.json"), &sec); err != nil {
		return err
	}
	op, err := wallet.FromPrivateKeyHex(sec.OperatorPrivateHex)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"validator_id": m.Nodes[target].ValidatorID})
	cfg := blockchain.DefaultProtocolV2Config()
	fee, _ := cfg.FeePolicy.ComputeFee(validatorUnbondCompute)
	seq, err := querySequence(m, op.Address())
	if err != nil {
		return err
	}
	tx := protocolv2.NewTransactionV2(m.ChainID, protocolv2.TxValidatorUnbond, op.Address(), "", 0, seq, validatorUnbondCompute, fee, payload)
	if err := tx.Sign(op); err != nil {
		return err
	}
	r, err := submitAndWait(m, tx, 25*time.Second)
	if err != nil {
		return err
	}
	if !r.Success {
		return fmt.Errorf("unbond receipt reverted: %s", r.RevertReason)
	}
	fmt.Printf("HVM_NETWORK_UNBOND_OK target=%d validator=%s tx=0x%s\n", target, m.Nodes[target].ValidatorID, tx.HashHex())
	return nil
}

func runStatus(m hvm_network.Manifest) error {
	for _, n := range m.Nodes {
		var h map[string]interface{}
		if err := httpJSON(fmt.Sprintf("http://%s:%d/health", n.IP, n.RPCPort), http.MethodGet, nil, &h); err != nil {
			fmt.Printf("node=%d ERROR %v\n", n.Index, err)
			continue
		}
		b, _ := json.Marshal(h)
		fmt.Printf("node=%d %s\n", n.Index, b)
	}
	return nil
}

func callContract(m hvm_network.Manifest, signer *wallet.Wallet, contract, method string, args interface{}, limit uint64) (hvm.Receipt, error) {
	argRaw, _ := json.Marshal(args)
	payloadRaw, _ := json.Marshal(contractCallPayload{Method: method, Args: argRaw})
	cfg := blockchain.DefaultProtocolV2Config()
	fee, _ := cfg.FeePolicy.MaxFeeForLimit(limit)
	seq, err := querySequence(m, signer.Address())
	if err != nil {
		return hvm.Receipt{}, err
	}
	tx := protocolv2.NewTransactionV2(m.ChainID, protocolv2.TxContractCall, signer.Address(), contract, 0, seq, limit, fee, payloadRaw)
	if err := tx.Sign(signer); err != nil {
		return hvm.Receipt{}, err
	}
	r, err := submitAndWait(m, tx, 25*time.Second)
	if err != nil {
		return r, err
	}
	if !r.Success {
		return r, fmt.Errorf("%s reverted: %s", method, r.RevertReason)
	}
	return r, nil
}

func getPayout(m hvm_network.Manifest, sender, contract, payoutID string) (hvm.PayoutCurrent, error) {
	args, _ := json.Marshal(map[string]string{"payout_id": payoutID})
	call := map[string]interface{}{"sender": sender, "address": contract, "method": "getPayout", "args": json.RawMessage(args), "compute_limit": 200000}
	var receipt hvm.Receipt
	if err := rpcResult(firstRPC(m), "hb_call", []interface{}{call}, &receipt); err != nil {
		return hvm.PayoutCurrent{}, err
	}
	if !receipt.Success {
		return hvm.PayoutCurrent{}, fmt.Errorf("getPayout reverted: %s", receipt.RevertReason)
	}
	var cur hvm.PayoutCurrent
	if err := json.Unmarshal(receipt.ReturnData, &cur); err != nil {
		return cur, err
	}
	return cur, nil
}

func submitAndWait(m hvm_network.Manifest, tx *protocolv2.TransactionV2, timeout time.Duration) (hvm.Receipt, error) {
	raw, err := tx.EncodeRaw()
	if err != nil {
		return hvm.Receipt{}, err
	}
	accepted := 0
	for _, n := range m.Nodes {
		url := fmt.Sprintf("http://%s:%d/rpc", n.IP, n.RPCPort)
		var txid string
		if err := rpcResult(url, "hb_sendRawTransactionV2", []interface{}{raw}, &txid); err == nil {
			accepted++
		}
	}
	if accepted == 0 {
		return hvm.Receipt{}, fmt.Errorf("transaction was not accepted by any devnet node")
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, n := range m.Nodes {
			url := fmt.Sprintf("http://%s:%d/rpc", n.IP, n.RPCPort)
			var receipt *hvm.Receipt
			if err := rpcResult(url, "hb_getTransactionReceipt", []interface{}{"0x" + tx.HashHex()}, &receipt); err == nil && receipt != nil {
				return *receipt, nil
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return hvm.Receipt{}, fmt.Errorf("receipt timeout for 0x%s", tx.HashHex())
}

func querySequence(m hvm_network.Manifest, address string) (uint64, error) {
	var raw string
	if err := rpcResult(firstRPC(m), "hb_getTransactionCount", []interface{}{address, "pending"}, &raw); err != nil {
		return 0, err
	}
	return parseHexUint(raw)
}

func nextValidatorSet(m hvm_network.Manifest) (uint64, string, error) {
	var status struct {
		HeadHeight int `json:"head_height"`
	}
	if err := rpcResult(firstRPC(m), "hb_getConsensusStatus", nil, &status); err != nil {
		return 0, "", err
	}
	height := uint64(status.HeadHeight + 1)
	var set struct {
		ValidatorSetRoot string `json:"validator_set_root"`
	}
	if err := rpcResult(firstRPC(m), "hb_getValidatorSet", []interface{}{height}, &set); err != nil {
		return 0, "", err
	}
	return height, set.ValidatorSetRoot, nil
}

func loadRecorder(out string) (*wallet.Wallet, error) {
	var sec hvm_network.RecorderSecret
	if err := hvm_network.LoadJSON(filepath.Join(out, "secrets", "recorder.json"), &sec); err != nil {
		return nil, err
	}
	return wallet.FromPrivateKeyHex(sec.PrivateHex)
}

func firstRPC(m hvm_network.Manifest) string {
	n := m.Nodes[0]
	return fmt.Sprintf("http://%s:%d/rpc", n.IP, n.RPCPort)
}

func rpcResult(url, method string, params []interface{}, out interface{}) error {
	if params == nil {
		params = []interface{}{}
	}
	body, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	var resp rpcResponse
	if err := httpJSON(url, http.MethodPost, body, &resp); err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("rpc %s: %s", method, resp.Error.Message)
	}
	if out == nil {
		return nil
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return nil
	}
	return json.Unmarshal(resp.Result, out)
}

func httpJSON(url, method string, body []byte, out interface{}) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func parseHexUint(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	base := 10
	if strings.HasPrefix(strings.ToLower(s), "0x") {
		base, s = 16, s[2:]
	}
	return strconv.ParseUint(s, base, 64)
}

func hashLabel(label string) string { return hashHex([]byte(label)) }
func hashHex(b []byte) string {
	h := sha256.Sum256(b)
	return "0x" + hex.EncodeToString(h[:])
}
func hmacBytes(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(msg)
	return h.Sum(nil)
}
func hmacHex(key, msg []byte) string { return "0x" + hex.EncodeToString(hmacBytes(key, msg)) }
func commitment(domain string, value, salt []byte) string {
	b := make([]byte, 0, len(domain)+2+len(value)+len(salt))
	b = append(b, []byte(domain)...)
	b = append(b, 0)
	b = append(b, value...)
	b = append(b, 0)
	b = append(b, salt...)
	return hashHex(b)
}
func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func must(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}
func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "hvm-network-ctl: "+format+"\n", args...)
	os.Exit(1)
}
