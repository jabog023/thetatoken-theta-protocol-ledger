package execution

import (
	"encoding/hex"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/thetatoken/ukulele/common"
	"github.com/thetatoken/ukulele/common/result"
	"github.com/thetatoken/ukulele/crypto"
	"github.com/thetatoken/ukulele/ledger/types"
)

func TestGetInputs(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	// nil submissions
	acc, res := getInputs(nil, nil)
	assert.True(res.IsOK(), "getInputs: error on nil submission")
	assert.Zero(len(acc), "getInputs: accounts returned on nil submission")

	// test getInputs for registered, non-registered account
	et.reset()
	inputs := types.Accs2TxInputs(1, et.accIn)
	acc, res = getInputs(et.state(), inputs)
	assert.True(res.IsError(), "getInputs: expected error when using getInput with non-registered Input")

	et.acc2State(et.accIn)
	acc, res = getInputs(et.state(), inputs)
	assert.True(res.IsOK(), "getInputs: expected to getInput from registered Input")

	// test sending duplicate accounts
	et.reset()
	et.acc2State(et.accIn, et.accIn, et.accIn)
	inputs = types.Accs2TxInputs(1, et.accIn, et.accIn, et.accIn)
	acc, res = getInputs(et.state(), inputs)
	assert.True(res.IsError(), "getInputs: expected error when sending duplicate accounts")

	// test calculating reward
	et.reset()
	et.acc2State(et.accIn)

	et.fastforwardBy(1000) // fastforward to reach a sufficient height for Gamma generation

	inputs = types.Accs2TxInputs(1, et.accIn)
	acc, res = getInputs(et.state(), inputs)
	assert.True(res.IsOK(), "getInputs: expected to get input from a few block heights ago")
	assert.True(acc[string(inputs[0].Address[:])].Balance.GetGammaWei().Amount > et.accIn.Balance.GetGammaWei().Amount,
		"getInputs: expected to update input account gamma balance")
}

func TestGetOrMakeOutputs(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	//nil submissions
	acc, res := getOrMakeOutputs(nil, nil, nil)
	assert.True(res.IsOK(), "getOrMakeOutputs: error on nil submission")
	assert.Zero(len(acc), "getOrMakeOutputs: accounts returned on nil submission")

	//test sending duplicate accounts
	et.reset()
	outputs := types.Accs2TxOutputs(et.accIn, et.accIn, et.accIn)
	_, res = getOrMakeOutputs(et.state(), nil, outputs)
	assert.True(res.IsError(), "getOrMakeOutputs: expected error when sending duplicate accounts")

	//test sending to existing/new account
	et.reset()
	outputs1 := types.Accs2TxOutputs(et.accIn)
	outputs2 := types.Accs2TxOutputs(et.accOut)

	et.acc2State(et.accIn)
	_, res = getOrMakeOutputs(et.state(), nil, outputs1)
	assert.True(res.IsOK(), "getOrMakeOutputs: error when sending to existing account")

	mapRes2, res := getOrMakeOutputs(et.state(), nil, outputs2)
	assert.True(res.IsOK(), "getOrMakeOutputs: error when sending to new account")

	//test the map results
	_, map2ok := mapRes2[string(outputs2[0].Address[:])]
	assert.True(map2ok, "getOrMakeOutputs: account output does not contain new account map item")

	//test calculating reward
	et.reset()
	et.fastforwardBy(1000) // fastforward to reach a sufficient height for Gamma generation

	outputs1 = types.Accs2TxOutputs(et.accIn)
	outputs2 = types.Accs2TxOutputs(et.accOut)

	et.acc2State(et.accIn)
	mapRes1, res := getOrMakeOutputs(et.state(), nil, outputs1)
	assert.True(res.IsOK(), "getOrMakeOutputs: error when sending to existing account")
	assert.True(mapRes1[string(outputs1[0].Address[:])].Balance.GetGammaWei().Amount > et.accIn.Balance.GetGammaWei().Amount,
		"getOrMakeOutputs: expected to update existing output account gamma balance")

	mapRes2, res = getOrMakeOutputs(et.state(), nil, outputs2)
	assert.True(res.IsOK(), "getOrMakeOutputs: error when sending to new account")
	assert.True(mapRes2[string(outputs2[0].Address[:])].Balance.GetGammaWei().Amount == 0,
		"getOrMakeOutputs: expected to not update new output account gamma balance")
}

func TestValidateInputsBasic(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	//validate input basic
	inputs := types.Accs2TxInputs(1, et.accIn)
	res := validateInputsBasic(inputs)
	assert.True(res.IsOK(), "validateInputsBasic: expected no error on good tx input. Error: %v", res.Message)

	t.Log("inputs[0].Coins = ", inputs[0].Coins)
	inputs[0].Coins[0].Amount = 0
	res = validateInputsBasic(inputs)
	//assert.True(res.IsError(), "validateInputsBasic: expected error on bad tx input")
	assert.True(res.IsOK(), "validateInputsBasic: expected error on bad tx input") // now inputs[0].Coins has two types of coins
}

func TestValidateInputsAdvanced(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	//create three temp accounts for the test
	accIn1 := types.MakeAcc("foox")
	accIn2 := types.MakeAcc("fooy")
	accIn3 := types.MakeAcc("fooz")

	//validate inputs advanced
	tx := types.MakeSendTx(1, et.accOut, accIn1, accIn2, accIn3)

	et.acc2State(accIn1, accIn2, accIn3, et.accOut)
	accMap, res := getInputs(et.state(), tx.Inputs)
	assert.True(res.IsOK(), "validateInputsAdvanced: error retrieving accMap. Error: %v", res.Message)
	signBytes := tx.SignBytes(et.chainID)

	//test bad case, unsigned
	totalCoins, res := validateInputsAdvanced(accMap, signBytes, tx.Inputs)
	assert.True(res.IsError(), "validateInputsAdvanced: expected an error on an unsigned tx input")

	//test good case sgined
	et.signSendTx(tx, accIn1, accIn2, accIn3, et.accOut)
	totalCoins, res = validateInputsAdvanced(accMap, signBytes, tx.Inputs)
	assert.True(res.IsOK(), "validateInputsAdvanced: expected no error on good tx input. Error: %v", res.Message)

	txTotalCoins := tx.Inputs[0].Coins.
		Plus(tx.Inputs[1].Coins).
		Plus(tx.Inputs[2].Coins)

	assert.True(totalCoins.IsEqual(txTotalCoins),
		"ValidateInputsAdvanced: transaction total coins are not equal: got %v, expected %v", txTotalCoins, totalCoins)
}

func TestValidateInputAdvanced(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	//validate input advanced
	tx := types.MakeSendTx(1, et.accOut, et.accIn)

	et.acc2State(et.accIn, et.accOut)
	signBytes := tx.SignBytes(et.chainID)

	//unsigned case
	res := validateInputAdvanced(&et.accIn.Account, signBytes, tx.Inputs[0])
	assert.True(res.IsError(), "validateInputAdvanced: expected error on tx input without signature")

	//good signed case
	et.signSendTx(tx, et.accIn, et.accOut)
	res = validateInputAdvanced(&et.accIn.Account, signBytes, tx.Inputs[0])
	assert.True(res.IsOK(), "validateInputAdvanced: expected no error on good tx input. Error: %v", res.Message)

	//bad sequence case
	et.accIn.Sequence = 1
	et.signSendTx(tx, et.accIn, et.accOut)
	res = validateInputAdvanced(&et.accIn.Account, signBytes, tx.Inputs[0])
	assert.Equal(result.CodeInvalidSequence, res.Code, "validateInputAdvanced: expected error on tx input with bad sequence")
	et.accIn.Sequence = 0 //restore sequence

	//bad balance case
	et.accIn.Balance = types.Coins{{Denom: "ThetaWei", Amount: 2}}
	et.signSendTx(tx, et.accIn, et.accOut)
	res = validateInputAdvanced(&et.accIn.Account, signBytes, tx.Inputs[0])
	assert.Equal(result.CodeInsufficientFund, res.Code,
		"validateInputAdvanced: expected error on tx input with insufficient funds %v", et.accIn.Sequence)
}

func TestValidateOutputsBasic(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	//validateOutputsBasic
	tx := types.Accs2TxOutputs(et.accIn)
	res := validateOutputsBasic(tx)
	assert.True(res.IsOK(), "validateOutputsBasic: expected no error on good tx output. Error: %v", res.Message)

	tx[0].Coins[0].Amount = 0
	res = validateOutputsBasic(tx)
	assert.True(res.IsError(), "validateInputBasic: expected error on bad tx output. Error: %v", res.Message)
}

func TestSumOutput(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	//SumOutput
	tx := types.Accs2TxOutputs(et.accIn, et.accOut)
	total := sumOutputs(tx)
	assert.True(total.IsEqual(tx[0].Coins.Plus(tx[1].Coins)), "sumOutputs: total coins are not equal")
}

func TestAdjustBy(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	//adjustByInputs/adjustByOutputs
	//sending transaction from accIn to accOut
	initBalIn := et.accIn.Account.Balance
	initBalOut := et.accOut.Account.Balance
	et.acc2State(et.accIn, et.accOut)

	txIn := types.Accs2TxInputs(1, et.accIn)
	txOut := types.Accs2TxOutputs(et.accOut)
	accMap, _ := getInputs(et.state(), txIn)
	accMap, _ = getOrMakeOutputs(et.state(), accMap, txOut)

	adjustByInputs(et.state(), accMap, txIn)
	adjustByOutputs(et.state(), accMap, txOut)

	inAddr := et.accIn.Account.PubKey.Address()
	outAddr := et.accOut.Account.PubKey.Address()
	endBalIn := accMap[string(inAddr[:])].Balance
	endBalOut := accMap[string(outAddr[:])].Balance
	decrBalIn := initBalIn.Minus(endBalIn)
	incrBalOut := endBalOut.Minus(initBalOut)

	assert.True(decrBalIn.IsEqual(txIn[0].Coins),
		"adjustByInputs: total coins are not equal. diff: %v, tx: %v", decrBalIn.String(), txIn[0].Coins.String())
	assert.True(incrBalOut.IsEqual(txOut[0].Coins),
		"adjustByInputs: total coins are not equal. diff: %v, tx: %v", incrBalOut.String(), txOut[0].Coins.String())
}

func TestSendTx(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	//ExecTx
	tx := types.MakeSendTx(1, et.accOut, et.accIn)
	et.acc2State(et.accIn)
	et.acc2State(et.accOut)
	et.signSendTx(tx, et.accIn)

	//Bad Balance
	et.accIn.Balance = types.Coins{{Denom: "ThetaWei", Amount: 2}}
	et.acc2State(et.accIn)
	res, _, _, _, _ := et.execSendTx(tx, true)
	assert.True(res.IsError(), "ExecTx/Bad CheckTx: Expected error return from ExecTx, returned: %v", res)

	res, balIn, balInExp, balOut, balOutExp := et.execSendTx(tx, false)
	assert.True(res.IsError(), "ExecTx/Bad DeliverTx: Expected error return from ExecTx, returned: %v", res)
	assert.False(balIn.IsEqual(balInExp),
		"ExecTx/Bad DeliverTx: balance shouldn't be equal for accIn: got %v, expected: %v", balIn, balInExp)
	assert.False(balOut.IsEqual(balOutExp),
		"ExecTx/Bad DeliverTx: balance shouldn't be equal for accOut: got %v, expected: %v", balOut, balOutExp)

	//Regular CheckTx
	et.reset()
	et.acc2State(et.accIn)
	et.acc2State(et.accOut)
	res, _, _, _, _ = et.execSendTx(tx, true)
	assert.True(res.IsOK(), "ExecTx/Good CheckTx: Expected OK return from ExecTx, Error: %v", res)

	//Regular DeliverTx
	et.reset()
	et.acc2State(et.accIn)
	et.acc2State(et.accOut)
	res, balIn, balInExp, balOut, balOutExp = et.execSendTx(tx, false)
	assert.True(res.IsOK(), "ExecTx/Good DeliverTx: Expected OK return from ExecTx, Error: %v", res)
	assert.True(balIn.IsEqual(balInExp),
		"ExecTx/good DeliverTx: unexpected change in input balance, got: %v, expected: %v", balIn, balInExp)
	assert.True(balOut.IsEqual(balOutExp),
		"ExecTx/good DeliverTx: unexpected change in output balance, got: %v, expected: %v", balOut, balOutExp)
}

func TestCalculateThetaReward(t *testing.T) {
	assert := assert.New(t)

	res := calculateThetaReward(1e17, true)
	assert.True(res.Amount > 0)
}

func TestNonEmptyPubKey(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	_, userPubKey, err := crypto.TEST_GenerateKeyPairWithSeed("user")
	assert.Nil(err)
	userAddr := userPubKey.Address()
	et.state().SetAccount(userAddr, &types.Account{
		LastUpdatedBlockHeight: 1,
	})

	// ----------- Test 1: Both acc.PubKey and txInput.PubKey are empty -----------

	accInit, res := getAccount(et.state(), userAddr)
	assert.True(res.IsOK())
	assert.Nil(accInit.PubKey)

	txInput1 := types.TxInput{
		Address:  userAddr,
		Sequence: 1,
	} // Empty PubKey

	acc, res := getInput(et.state(), txInput1)
	assert.Equal(result.CodeEmptyPubKeyWithSequence1, res.Code)
	assert.True(acc == nil)

	// ----------- Test 2: acc.PubKey is empty, and txInput.PubKey is not empty -----------

	accInit, res = getAccount(et.state(), userAddr)
	assert.True(res.IsOK())
	assert.Nil(accInit.PubKey)

	txInput2 := types.TxInput{
		Address:  userAddr,
		PubKey:   userPubKey,
		Sequence: 2,
	}

	acc, res = getInput(et.state(), txInput2)
	assert.True(res.IsOK())
	assert.False(acc.PubKey.IsEmpty())
	assert.Equal(acc.PubKey, userPubKey)

	// ----------- Test 3: acc.PubKey is not empty, but txInput.PubKey is empty -----------

	et.state().SetAccount(userAddr, &types.Account{
		PubKey:                 userPubKey,
		LastUpdatedBlockHeight: 1,
	})

	accInit, res = getAccount(et.state(), userAddr)
	assert.True(res.IsOK())
	assert.False(accInit.PubKey.IsEmpty())

	txInput3 := types.TxInput{
		Address:  userAddr,
		Sequence: 3,
	} // Empty PubKey

	acc, res = getInput(et.state(), txInput3)
	assert.True(res.IsOK())
	assert.False(acc.PubKey.IsEmpty())
	assert.Equal(acc.PubKey, userPubKey)

	// ----------- Test 4: txInput contains another PubKey, should be ignored -----------

	_, userPubKey2, err := crypto.TEST_GenerateKeyPairWithSeed("lol")
	assert.Nil(err)

	accInit, res = getAccount(et.state(), userAddr)
	assert.True(res.IsOK())
	assert.False(accInit.PubKey.IsEmpty())

	txInput4 := types.TxInput{
		Address:  userAddr,
		Sequence: 4,
		PubKey:   userPubKey2,
	}

	acc, res = getInput(et.state(), txInput4)
	assert.True(res.IsOK())
	assert.False(acc.PubKey.IsEmpty())
	assert.Equal(userPubKey, acc.PubKey)     // acc.PukKey should not change
	assert.NotEqual(userPubKey2, acc.PubKey) // acc.PukKey should not change
}

func TestCoinbaseTx(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	va1 := et.accProposer
	va1.Balance = types.Coins{{"ThetaWei", 1e11}}
	et.acc2State(va1)

	va2 := et.accVal2
	va2.Balance = types.Coins{{"ThetaWei", 3e11}}
	et.acc2State(va2)

	user1 := types.MakeAcc("user 1")
	user1.Balance = types.Coins{{"ThetaWei", 1e11}}
	et.acc2State(user1)

	et.fastforwardTo(1e7)

	var tx *types.CoinbaseTx
	var res result.Result

	//Regular check
	tx = &types.CoinbaseTx{
		Proposer: types.TxInput{
			Address: va1.PubKey.Address(), PubKey: va1.PubKey},
		Outputs: []types.TxOutput{{
			va1.Account.PubKey.Address(), types.Coins{{"ThetaWei", 317}},
		}, {
			va2.Account.PubKey.Address(), types.Coins{{"ThetaWei", 951}},
		}},
		BlockHeight: 1e7,
	}
	tx.Proposer.Signature = va1.Sign(tx.SignBytes(et.chainID))

	res = et.executor.getTxExecutor(tx).sanityCheck(et.chainID, et.state().Delivered(), tx)
	assert.True(res.IsOK(), res.String())

	//Error if reward Theta amount is incorrect
	tx = &types.CoinbaseTx{
		Proposer: types.TxInput{
			Address: va1.PubKey.Address(), PubKey: va1.PubKey},
		Outputs: []types.TxOutput{{
			va1.Account.PubKey.Address(), types.Coins{{"ThetaWei", 317}},
		}, {
			va2.Account.PubKey.Address(), types.Coins{{"ThetaWei", 317}},
		}},
		BlockHeight: 1e7,
	}
	res = et.executor.getTxExecutor(tx).sanityCheck(et.chainID, et.state().Delivered(), tx)
	assert.True(res.IsError(), res.String())

	//Error if reward Gamma amount is incorrect
	tx = &types.CoinbaseTx{
		Proposer: types.TxInput{
			Address: va1.PubKey.Address(), PubKey: va1.PubKey},
		Outputs: []types.TxOutput{{
			va1.Account.PubKey.Address(), types.Coins{{"ThetaWei", 317}},
		}, {
			va2.Account.PubKey.Address(), types.Coins{{"ThetaWei", 951}, {"GammaWei", 1}},
		}},
		BlockHeight: 1e7,
	}
	res = et.executor.getTxExecutor(tx).sanityCheck(et.chainID, et.state().Delivered(), tx)
	assert.True(res.IsError(), res.String())

	//Error if Validator 2 is not rewarded
	tx = &types.CoinbaseTx{
		Proposer: types.TxInput{
			Address: va1.PubKey.Address(), PubKey: va1.PubKey},
		Outputs: []types.TxOutput{{
			va1.Account.PubKey.Address(), types.Coins{{"ThetaWei", 317}},
		}},
		BlockHeight: 1e7,
	}
	res = et.executor.getTxExecutor(tx).sanityCheck(et.chainID, et.state().Delivered(), tx)
	assert.True(res.IsError(), res.String())

	//Error if non-validator is rewarded
	tx = &types.CoinbaseTx{
		Proposer: types.TxInput{
			Address: va1.PubKey.Address(), PubKey: va1.PubKey},
		Outputs: []types.TxOutput{{
			va1.Account.PubKey.Address(), types.Coins{{"ThetaWei", 317}},
		}, {
			va2.Account.PubKey.Address(), types.Coins{{"ThetaWei", 951}},
		}, {
			user1.Account.PubKey.Address(), types.Coins{{"ThetaWei", 317}},
		}},
		BlockHeight: 1e7,
	}
	res = et.executor.getTxExecutor(tx).sanityCheck(et.chainID, et.state().Delivered(), tx)
	assert.True(res.IsError(), res.String())

	//Error if validator address is changed
	tx = &types.CoinbaseTx{
		Proposer: types.TxInput{
			Address: va1.PubKey.Address(), PubKey: va1.PubKey},
		Outputs: []types.TxOutput{{
			va1.Account.PubKey.Address(), types.Coins{{"ThetaWei", 317}},
		}, {
			user1.Account.PubKey.Address(), types.Coins{{"ThetaWei", 317}},
		}},
		BlockHeight: 1e7,
	}
	res = et.executor.getTxExecutor(tx).sanityCheck(et.chainID, et.state().Delivered(), tx)
	assert.True(res.IsError(), res.String())

	//Process should update validator account
	tx = &types.CoinbaseTx{
		Proposer: types.TxInput{
			Address: va1.PubKey.Address(), PubKey: va1.PubKey},
		Outputs: []types.TxOutput{{
			va1.Account.PubKey.Address(), types.Coins{{"ThetaWei", 317}},
		}, {
			va2.Account.PubKey.Address(), types.Coins{{"ThetaWei", 951}},
		}},
		BlockHeight: 1e7,
	}

	_, res = et.executor.getTxExecutor(tx).process(et.chainID, et.state().Delivered(), tx)
	assert.True(res.IsOK(), res.String())

	va1balance := et.state().GetAccount(va1.Account.PubKey.Address()).Balance
	assert.Equal(int64(100000000317), va1balance.GetThetaWei().Amount)
	// validator's Gamma is also updated.
	assert.Equal(int64(189999981000), va1balance.GetGammaWei().Amount)

	va2balance := et.state().GetAccount(va2.Account.PubKey.Address()).Balance
	assert.Equal(int64(300000000951), va2balance.GetThetaWei().Amount)
	assert.Equal(int64(569999943000), va2balance.GetGammaWei().Amount)

	user1balance := et.state().GetAccount(user1.Account.PubKey.Address()).Balance
	assert.Equal(int64(100000000000), user1balance.GetThetaWei().Amount)
	// user's Gamma is not updated.
	assert.Equal(int64(0), user1balance.GetGammaWei().Amount)
}

func TestReserveFundTx(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	user1 := types.MakeAcc("user 1")
	user1.Balance = types.Coins{{"GammaWei", 10000 * 1e6}, {"ThetaWei", 10000 * 1e6}}
	et.acc2State(user1)

	et.fastforwardTo(1e7)

	var tx *types.ReserveFundTx
	var res result.Result

	// Invalid Fee
	tx = &types.ReserveFundTx{
		Fee: types.Coin{"foo", 100},
		Source: types.TxInput{
			Address:  user1.PubKey.Address(),
			PubKey:   user1.PubKey,
			Coins:    types.Coins{{"GammaWei", 1000 * 1e6}},
			Sequence: 1,
		},
		Collateral:  types.Coins{{"GammaWei", 1001 * 1e6}},
		ResourceIDs: []common.Bytes{common.Bytes("rid001")},
		Duration:    1000,
	}
	tx.Source.Signature = user1.Sign(tx.SignBytes(et.chainID))
	res = et.executor.getTxExecutor(tx).sanityCheck(et.chainID, et.state().Delivered(), tx)
	assert.False(res.IsOK(), res.String())
	assert.Equal(res.Code, result.CodeInvalidFee)

	// Reserved fund not specified
	tx = &types.ReserveFundTx{
		Fee: types.Coin{"GammaWei", 100},
		Source: types.TxInput{
			Address:  user1.PubKey.Address(),
			PubKey:   user1.PubKey,
			Sequence: 1,
		},
		Collateral:  types.Coins{{"GammaWei", 1001 * 1e6}},
		ResourceIDs: []common.Bytes{common.Bytes("rid001")},
		Duration:    1000,
	}
	tx.Source.Signature = user1.Sign(tx.SignBytes(et.chainID))
	res = et.executor.getTxExecutor(tx).sanityCheck(et.chainID, et.state().Delivered(), tx)
	assert.False(res.IsOK(), res.String())
	assert.Equal(res.Code, result.CodeReservedFundNotSpecified)

	// Insufficient fund
	tx = &types.ReserveFundTx{
		Fee: types.Coin{"GammaWei", 100},
		Source: types.TxInput{
			Address:  user1.PubKey.Address(),
			PubKey:   user1.PubKey,
			Coins:    types.Coins{{"GammaWei", 50000 * 1e6}},
			Sequence: 1,
		},
		Collateral:  types.Coins{{"GammaWei", 1001 * 1e6}},
		ResourceIDs: []common.Bytes{common.Bytes("rid001")},
		Duration:    1000,
	}
	tx.Source.Signature = user1.Sign(tx.SignBytes(et.chainID))
	res = et.executor.getTxExecutor(tx).sanityCheck(et.chainID, et.state().Delivered(), tx)
	assert.False(res.IsOK(), res.String())
	assert.Equal(res.Code, result.CodeInsufficientFund)

	// Reserved fund more than collateral
	tx = &types.ReserveFundTx{
		Fee: types.Coin{"GammaWei", 100},
		Source: types.TxInput{
			Address:  user1.PubKey.Address(),
			PubKey:   user1.PubKey,
			Coins:    types.Coins{{"GammaWei", 5000 * 1e6}},
			Sequence: 1,
		},
		Collateral:  types.Coins{{"GammaWei", 1001 * 1e6}},
		ResourceIDs: []common.Bytes{common.Bytes("rid001")},
		Duration:    1000,
	}
	tx.Source.Signature = user1.Sign(tx.SignBytes(et.chainID))
	res = et.executor.getTxExecutor(tx).sanityCheck(et.chainID, et.state().Delivered(), tx)
	assert.False(res.IsOK(), res.String())
	assert.Equal(res.Code, result.CodeReserveFundCheckFailed)

	// Regular check
	tx = &types.ReserveFundTx{
		Fee: types.Coin{"GammaWei", 100},
		Source: types.TxInput{
			Address:  user1.PubKey.Address(),
			PubKey:   user1.PubKey,
			Coins:    types.Coins{{"GammaWei", 1000 * 1e6}},
			Sequence: 1,
		},
		Collateral:  types.Coins{{"GammaWei", 1001 * 1e6}},
		ResourceIDs: []common.Bytes{common.Bytes("rid001")},
		Duration:    1000,
	}
	tx.Source.Signature = user1.Sign(tx.SignBytes(et.chainID))
	res = et.executor.getTxExecutor(tx).sanityCheck(et.chainID, et.state().Delivered(), tx)
	assert.True(res.IsOK(), res.String())
	_, res = et.executor.getTxExecutor(tx).process(et.chainID, et.state().Delivered(), tx)
	assert.True(res.IsOK(), res.String())

	retrievedUserAcc := et.state().GetAccount(user1.PubKey.Address())
	assert.Equal(1, len(retrievedUserAcc.ReservedFunds))
	assert.Equal([]common.Bytes{common.Bytes("rid001")}, retrievedUserAcc.ReservedFunds[0].ResourceIDs)
	assert.Equal(types.Coins{{"GammaWei", 1001 * 1e6}}, retrievedUserAcc.ReservedFunds[0].Collateral)
	assert.Equal(1, retrievedUserAcc.ReservedFunds[0].ReserveSequence)
}

func TestReleaseFundTx(t *testing.T) {
	assert := assert.New(t)
	et := newExecTest()

	user1 := types.MakeAcc("user 1")
	user1.Balance = types.Coins{{"GammaWei", 10000 * 1e6}, {"ThetaWei", 10000 * 1e6}}
	et.acc2State(user1)

	et.fastforwardTo(1e7)

	var reserveFundTx *types.ReserveFundTx
	var releaseFundTx *types.ReleaseFundTx
	var res result.Result

	reserveFundTx = &types.ReserveFundTx{
		Fee: types.Coin{"GammaWei", 100},
		Source: types.TxInput{
			Address:  user1.PubKey.Address(),
			PubKey:   user1.PubKey,
			Coins:    types.Coins{{"GammaWei", 1000 * 1e6}},
			Sequence: 1,
		},
		Collateral:  types.Coins{{"GammaWei", 1001 * 1e6}},
		ResourceIDs: []common.Bytes{common.Bytes("rid001")},
		Duration:    1000,
	}
	reserveFundTx.Source.Signature = user1.Sign(reserveFundTx.SignBytes(et.chainID))
	res = et.executor.getTxExecutor(reserveFundTx).sanityCheck(et.chainID, et.state().Delivered(), reserveFundTx)
	assert.True(res.IsOK(), res.String())
	_, res = et.executor.getTxExecutor(reserveFundTx).process(et.chainID, et.state().Delivered(), reserveFundTx)
	assert.True(res.IsOK(), res.String())

	et.state().Commit()

	// Invalid Fee
	releaseFundTx = &types.ReleaseFundTx{
		Fee: types.Coin{"foo", 100},
		Source: types.TxInput{
			Address:  user1.PubKey.Address(),
			Sequence: 2,
		},
		ReserveSequence: 1,
	}
	releaseFundTx.Source.Signature = user1.Sign(releaseFundTx.SignBytes(et.chainID))
	res = et.executor.getTxExecutor(releaseFundTx).sanityCheck(et.chainID, et.state().Delivered(), releaseFundTx)
	assert.False(res.IsOK(), res.String())
	assert.Equal(res.Code, result.CodeInvalidFee, res.String())

	// Not expire yet
	releaseFundTx = &types.ReleaseFundTx{
		Fee: types.Coin{"GammaWei", 100},
		Source: types.TxInput{
			Address:  user1.PubKey.Address(),
			Sequence: 2,
		},
		ReserveSequence: 1,
	}
	releaseFundTx.Source.Signature = user1.Sign(releaseFundTx.SignBytes(et.chainID))
	res = et.executor.getTxExecutor(releaseFundTx).sanityCheck(et.chainID, et.state().Delivered(), releaseFundTx)
	assert.False(res.IsOK(), res.String())
	assert.Equal(res.Code, result.CodeReleaseFundCheckFailed, res.String())

	et.fastforwardTo(1e7 + 9999)

	// No matching ReserveSequence
	releaseFundTx = &types.ReleaseFundTx{
		Fee: types.Coin{"GammaWei", 100},
		Source: types.TxInput{
			Address:  user1.PubKey.Address(),
			Sequence: 2,
		},
		ReserveSequence: 99,
	}
	releaseFundTx.Source.Signature = user1.Sign(releaseFundTx.SignBytes(et.chainID))
	res = et.executor.getTxExecutor(releaseFundTx).sanityCheck(et.chainID, et.state().Delivered(), releaseFundTx)
	assert.False(res.IsOK(), res.String())
	assert.Equal(res.Code, result.CodeReleaseFundCheckFailed, res.String())

	// NOTE: The following check should FAIL, since the expired ReservedFunds are now
	//       released by the Account.UpdateToHeight() function. Once the height
	//       reaches the target release height, the ReservedFunds will be released
	//       automatically. No need to explicitly execute ReleaseFundTx

	// Check auto-expiration
	releaseFundTx = &types.ReleaseFundTx{
		Fee: types.Coin{"GammaWei", 100},
		Source: types.TxInput{
			Address:  user1.PubKey.Address(),
			Sequence: 2,
		},
		ReserveSequence: 1,
	}
	releaseFundTx.Source.Signature = user1.Sign(releaseFundTx.SignBytes(et.chainID))
	res = et.executor.getTxExecutor(releaseFundTx).sanityCheck(et.chainID, et.state().Delivered(), releaseFundTx)
	assert.False(res.IsOK(), res.String())
	assert.Equal(res.Code, result.CodeReleaseFundCheckFailed, res.String())
}

func TestServicePaymentTxNormalExecutionAndSlash(t *testing.T) {
	assert := assert.New(t)
	et, resourceID, alice, bob, carol, _, bobInitBalance, carolInitBalance := setupForServicePayment(assert)
	et.state().Commit()

	retrievedAliceAcc0 := et.state().GetAccount(alice.PubKey.Address())
	assert.Equal(1, len(retrievedAliceAcc0.ReservedFunds))
	assert.Equal([]common.Bytes{resourceID}, retrievedAliceAcc0.ReservedFunds[0].ResourceIDs)
	assert.Equal(types.Coins{{"GammaWei", 1001 * 1e6}}, retrievedAliceAcc0.ReservedFunds[0].Collateral)
	assert.Equal(1, retrievedAliceAcc0.ReservedFunds[0].ReserveSequence)

	// Simulate micropayment #1 between Alice and Bob
	payAmount1 := int64(800)
	srcSeq, tgtSeq, paymentSeq, reserveSeq := 1, 1, 1, 1
	_ = createServicePaymentTx(et.chainID, &alice, &bob, 100, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	_ = createServicePaymentTx(et.chainID, &alice, &bob, 500, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	servicePaymentTx1 := createServicePaymentTx(et.chainID, &alice, &bob, payAmount1, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	res := et.executor.getTxExecutor(servicePaymentTx1).sanityCheck(et.chainID, et.state().Delivered(), servicePaymentTx1)
	assert.True(res.IsOK(), res.Message)
	_, res = et.executor.getTxExecutor(servicePaymentTx1).process(et.chainID, et.state().Delivered(), servicePaymentTx1)
	assert.True(res.IsOK(), res.Message)
	assert.Equal(0, len(et.state().GetSlashIntents()))

	et.state().Commit()

	retrievedAliceAcc1 := et.state().GetAccount(alice.PubKey.Address())
	assert.Equal(types.Coins{{"GammaWei", payAmount1}}, retrievedAliceAcc1.ReservedFunds[0].UsedFund)
	retrievedBobAcc1 := et.state().GetAccount(bob.PubKey.Address())
	assert.Equal(bobInitBalance.Plus(types.Coins{{"GammaWei", payAmount1 - 1}}), retrievedBobAcc1.Balance) // payAmount1 - 1: need to account for Gas

	// Simulate micropayment #2 between Alice and Bob
	payAmount2 := int64(500)
	srcSeq, tgtSeq, paymentSeq, reserveSeq = 1, 2, 2, 1
	_ = createServicePaymentTx(et.chainID, &alice, &bob, 300, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	servicePaymentTx2 := createServicePaymentTx(et.chainID, &alice, &bob, payAmount2, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	res = et.executor.getTxExecutor(servicePaymentTx2).sanityCheck(et.chainID, et.state().Delivered(), servicePaymentTx2)
	assert.True(res.IsOK(), res.Message)
	_, res = et.executor.getTxExecutor(servicePaymentTx2).process(et.chainID, et.state().Delivered(), servicePaymentTx2)
	assert.True(res.IsOK(), res.Message)
	assert.Equal(0, len(et.state().GetSlashIntents()))

	et.state().Commit()

	retrievedAliceAcc2 := et.state().GetAccount(alice.PubKey.Address())
	assert.Equal(types.Coins{{"GammaWei", payAmount1 + payAmount2}}, retrievedAliceAcc2.ReservedFunds[0].UsedFund)
	retrievedBobAcc2 := et.state().GetAccount(bob.PubKey.Address())
	assert.Equal(bobInitBalance.Plus(types.Coins{{"GammaWei", payAmount1 + payAmount2 - 2}}), retrievedBobAcc2.Balance) // payAmount1 + payAmount2 - 2: need to account for Gas

	// Simulate micropayment #3 between Alice and Carol
	payAmount3 := int64(1200)
	srcSeq, tgtSeq, paymentSeq, reserveSeq = 1, 1, 3, 1
	_ = createServicePaymentTx(et.chainID, &alice, &carol, 300, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	servicePaymentTx3 := createServicePaymentTx(et.chainID, &alice, &carol, payAmount3, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	res = et.executor.getTxExecutor(servicePaymentTx3).sanityCheck(et.chainID, et.state().Delivered(), servicePaymentTx3)
	assert.True(res.IsOK(), res.Message)
	_, res = et.executor.getTxExecutor(servicePaymentTx3).process(et.chainID, et.state().Delivered(), servicePaymentTx3)
	assert.True(res.IsOK(), res.Message)
	assert.Equal(0, len(et.state().GetSlashIntents()))

	et.state().Commit()

	retrievedAliceAcc3 := et.state().GetAccount(alice.PubKey.Address())
	assert.Equal(types.Coins{{"GammaWei", payAmount1 + payAmount2 + payAmount3}}, retrievedAliceAcc3.ReservedFunds[0].UsedFund)
	retrievedCarolAcc3 := et.state().GetAccount(carol.PubKey.Address())
	assert.Equal(carolInitBalance.Plus(types.Coins{{"GammaWei", payAmount3 - 1}}), retrievedCarolAcc3.Balance) // payAmount3 - 1: need to account for Gas

	// Simulate micropayment #4 between Alice and Carol. This is an overspend, alice should get slashed.
	payAmount4 := int64(2000 * 1e6)
	srcSeq, tgtSeq, paymentSeq, reserveSeq = 1, 2, 4, 1
	_ = createServicePaymentTx(et.chainID, &alice, &carol, 70000, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	servicePaymentTx4 := createServicePaymentTx(et.chainID, &alice, &carol, payAmount4, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	res = et.executor.getTxExecutor(servicePaymentTx4).sanityCheck(et.chainID, et.state().Delivered(), servicePaymentTx4)
	assert.True(res.IsOK(), res.Message) // the following process() call will create an SlashIntent

	assert.Equal(0, len(et.state().GetSlashIntents()))
	_, res = et.executor.getTxExecutor(servicePaymentTx4).process(et.chainID, et.state().Delivered(), servicePaymentTx4)
	assert.True(res.IsOK(), res.Message)
	assert.Equal(1, len(et.state().GetSlashIntents()))
}

func TestServicePaymentTxExpiration(t *testing.T) {
	assert := assert.New(t)
	et, resourceID, alice, bob, _, _, bobInitBalance, _ := setupForServicePayment(assert)
	et.state().Commit()

	retrievedAliceAcc1 := et.state().GetAccount(alice.PubKey.Address())
	assert.Equal(1, len(retrievedAliceAcc1.ReservedFunds))
	assert.Equal([]common.Bytes{resourceID}, retrievedAliceAcc1.ReservedFunds[0].ResourceIDs)
	assert.Equal(types.Coins{{"GammaWei", 1001 * 1e6}}, retrievedAliceAcc1.ReservedFunds[0].Collateral)
	assert.Equal(1, retrievedAliceAcc1.ReservedFunds[0].ReserveSequence)

	// Simulate micropayment #1 between Alice and Bobs
	payAmount1 := int64(800)
	srcSeq, tgtSeq, paymentSeq, reserveSeq := 1, 1, 1, 1
	_ = createServicePaymentTx(et.chainID, &alice, &bob, 100, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	_ = createServicePaymentTx(et.chainID, &alice, &bob, 500, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	servicePaymentTx1 := createServicePaymentTx(et.chainID, &alice, &bob, payAmount1, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	res := et.executor.getTxExecutor(servicePaymentTx1).sanityCheck(et.chainID, et.state().Delivered(), servicePaymentTx1)
	assert.True(res.IsOK(), res.Message)
	_, res = et.executor.getTxExecutor(servicePaymentTx1).process(et.chainID, et.state().Delivered(), servicePaymentTx1)
	assert.True(res.IsOK(), res.Message)

	et.state().Commit()

	retrievedAliceAcc2 := et.state().GetAccount(alice.PubKey.Address())
	assert.Equal(types.Coins{{"GammaWei", payAmount1}}, retrievedAliceAcc2.ReservedFunds[0].UsedFund)
	retrievedBobAcc2 := et.state().GetAccount(bob.PubKey.Address())
	assert.Equal(bobInitBalance.Plus(types.Coins{{"GammaWei", payAmount1 - 1}}), retrievedBobAcc2.Balance) // payAmount1 - 1: need to account for Gas

	et.fastforwardBy(1e4) // The reservedFund should expire after the fastforward

	// Simulate micropayment #2 between Alice and Bobs
	payAmount2 := int64(500)
	srcSeq, tgtSeq, paymentSeq, reserveSeq = 1, 2, 2, 1
	_ = createServicePaymentTx(et.chainID, &alice, &bob, 300, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	servicePaymentTx2 := createServicePaymentTx(et.chainID, &alice, &bob, payAmount2, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	res = et.executor.getTxExecutor(servicePaymentTx2).sanityCheck(et.chainID, et.state().Delivered(), servicePaymentTx2)
	assert.False(res.IsOK(), res.Message)
	assert.Equal(result.CodeCheckTransferReservedFundFailed, res.Code)
	log.Infof("Service payment check message: %v", res.Message)
}

func TestSlashTx(t *testing.T) {
	assert := assert.New(t)
	et, resourceID, alice, bob, _, _, _, _ := setupForServicePayment(assert)

	proposer := et.accProposer
	proposerInitBalance := proposer.Account.Balance
	et.acc2State(proposer)
	log.Infof("Proposer's pubKey:  %v", hex.EncodeToString(proposer.PubKey.ToBytes()))
	log.Infof("Proposer's Address: %v", proposer.PubKey.Address().Hex())

	et.state().Commit()

	retrievedAliceAccount := et.state().GetAccount(alice.PubKey.Address())
	assert.Equal(1, len(retrievedAliceAccount.ReservedFunds))
	aliceCollateral := retrievedAliceAccount.ReservedFunds[0].Collateral
	aliceReservedFund := retrievedAliceAccount.ReservedFunds[0].InitialFund
	expectedAliceSlashedAmount := aliceCollateral.Plus(aliceReservedFund)

	// Simulate micropayment #1 between Alice and Bob, which is an overspend
	payAmount1 := int64(8000 * 1e6)
	srcSeq, tgtSeq, paymentSeq, reserveSeq := 1, 1, 1, 1
	_ = createServicePaymentTx(et.chainID, &alice, &bob, 100, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	_ = createServicePaymentTx(et.chainID, &alice, &bob, 500, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	servicePaymentTx1 := createServicePaymentTx(et.chainID, &alice, &bob, payAmount1, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	res := et.executor.getTxExecutor(servicePaymentTx1).sanityCheck(et.chainID, et.state().Delivered(), servicePaymentTx1)
	assert.True(res.IsOK(), res.Message)

	assert.Equal(0, len(et.state().GetSlashIntents()))
	_, res = et.executor.getTxExecutor(servicePaymentTx1).process(et.chainID, et.state().Delivered(), servicePaymentTx1)
	assert.True(res.IsOK(), res.Message)
	assert.Equal(1, len(et.state().GetSlashIntents()))

	slashIntent := et.state().GetSlashIntents()[0]

	et.state().Commit()

	// Test the slashTx
	slashTx := &types.SlashTx{
		Proposer: types.TxInput{
			Address:  proposer.PubKey.Address(),
			Sequence: 1,
			PubKey:   proposer.PubKey,
		},
		SlashedAddress:  slashIntent.Address,
		ReserveSequence: slashIntent.ReserveSequence,
		SlashProof:      slashIntent.Proof,
	}
	signBytes := slashTx.SignBytes(et.chainID)
	slashTx.Proposer.Signature = proposer.Sign(signBytes)

	res = et.executor.getTxExecutor(slashTx).sanityCheck(et.chainID, et.state().Delivered(), slashTx)
	assert.True(res.IsOK(), res.Message)
	_, res = et.executor.getTxExecutor(slashTx).process(et.chainID, et.state().Delivered(), slashTx)
	assert.True(res.IsOK(), res.Message)

	retrievedProposerAccount := et.state().GetAccount(proposer.PubKey.Address())
	assert.Equal(proposerInitBalance.Plus(expectedAliceSlashedAmount), retrievedProposerAccount.Balance) // slashed tokens transferred to the proposer

	retrievedAliceAccountAfterSlash := et.state().GetAccount(alice.PubKey.Address())
	assert.Equal(0, len(retrievedAliceAccountAfterSlash.ReservedFunds)) // Alice is slashed

	log.Infof("Proposer initial balance: %v", proposerInitBalance)
	log.Infof("Alice collateral: %v", aliceCollateral)
	log.Infof("Alice reserved fund: %v", aliceReservedFund)
	log.Infof("Proposer final balance: %v", retrievedProposerAccount.Balance)
}

func TestSplitContractTxNormalExecution(t *testing.T) {
	assert := assert.New(t)
	et, resourceID, alice, bob, carol, _, bobInitBalance, carolInitBalance := setupForServicePayment(assert)
	log.Infof("Bob's initial balance:   %v", bobInitBalance)
	log.Infof("Carol's initial balance: %v", carolInitBalance)

	initiator := types.MakeAcc("User David")
	initiator.Balance = types.Coins{{"GammaWei", 10000 * 1e6}}
	et.acc2State(initiator)

	splitCarol := types.Split{
		Address:    carol.PubKey.Address(),
		Percentage: 30,
	}
	splitContractTx := &types.SplitContractTx{
		Fee:        types.Coin{"GammaWei", 10},
		ResourceID: resourceID,
		Initiator: types.TxInput{
			Address:  initiator.PubKey.Address(),
			PubKey:   initiator.PubKey,
			Sequence: 1,
		},
		Splits:   []types.Split{splitCarol},
		Duration: uint32(99999),
	}
	signBytes := splitContractTx.SignBytes(et.chainID)
	splitContractTx.Initiator.Signature = initiator.Sign(signBytes)

	res := et.executor.getTxExecutor(splitContractTx).sanityCheck(et.chainID, et.state().Delivered(), splitContractTx)
	assert.True(res.IsOK(), res.Message)
	_, res = et.executor.getTxExecutor(splitContractTx).process(et.chainID, et.state().Delivered(), splitContractTx)
	assert.True(res.IsOK(), res.Message)

	// Simulate micropayment #1 between Alice and Bob, Carol should get a cut
	payAmount := int64(10000)
	srcSeq, tgtSeq, paymentSeq, reserveSeq := 1, 1, 1, 1
	_ = createServicePaymentTx(et.chainID, &alice, &bob, 100, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	_ = createServicePaymentTx(et.chainID, &alice, &bob, 500, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	servicePaymentTx := createServicePaymentTx(et.chainID, &alice, &bob, payAmount, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	res = et.executor.getTxExecutor(servicePaymentTx).sanityCheck(et.chainID, et.state().Delivered(), servicePaymentTx)
	assert.True(res.IsOK(), res.Message)

	assert.Equal(0, len(et.state().GetSlashIntents()))
	_, res = et.executor.getTxExecutor(servicePaymentTx).process(et.chainID, et.state().Delivered(), servicePaymentTx)
	assert.True(res.IsOK(), res.Message)

	et.state().Commit()

	bobFinalBalance := et.state().GetAccount(bob.PubKey.Address()).Balance
	carolFinalBalance := et.state().GetAccount(carol.PubKey.Address()).Balance
	log.Infof("Bob's final balance:   %v", bobFinalBalance)
	log.Infof("Carol's final balance: %v", carolFinalBalance)

	// Check the balances of the relevant accounts
	bobSplitCoins := types.Coins{types.Coin{"GammaWei", int64(payAmount * 70 / 100)}}
	servicePaymentTxFee := types.Coins{types.Coin{"GammaWei", 1}}
	carolSplitCoins := types.Coins{types.Coin{"GammaWei", int64(payAmount * 30 / 100)}}
	assert.Equal(bobInitBalance.Plus(bobSplitCoins).Minus(servicePaymentTxFee), bobFinalBalance)
	assert.Equal(carolInitBalance.Plus(carolSplitCoins), carolFinalBalance)
}

func TestSplitContractTxExpiration(t *testing.T) {
	assert := assert.New(t)
	et, resourceID, alice, bob, carol, _, bobInitBalance, carolInitBalance := setupForServicePayment(assert)
	log.Infof("Bob's initial balance:   %v", bobInitBalance)
	log.Infof("Carol's initial balance: %v", carolInitBalance)

	initiator := types.MakeAcc("User David")
	initiator.Balance = types.Coins{{"GammaWei", 10000 * 1e6}}
	et.acc2State(initiator)

	splitCarol := types.Split{
		Address:    carol.PubKey.Address(),
		Percentage: 30,
	}
	splitContractTx := &types.SplitContractTx{
		Fee:        types.Coin{"GammaWei", 10},
		ResourceID: resourceID,
		Initiator: types.TxInput{
			Address:  initiator.PubKey.Address(),
			PubKey:   initiator.PubKey,
			Sequence: 1,
		},
		Splits:   []types.Split{splitCarol},
		Duration: uint32(100),
	}
	signBytes := splitContractTx.SignBytes(et.chainID)
	splitContractTx.Initiator.Signature = initiator.Sign(signBytes)

	res := et.executor.getTxExecutor(splitContractTx).sanityCheck(et.chainID, et.state().Delivered(), splitContractTx)
	assert.True(res.IsOK(), res.Message)
	_, res = et.executor.getTxExecutor(splitContractTx).process(et.chainID, et.state().Delivered(), splitContractTx)
	assert.True(res.IsOK(), res.Message)

	et.fastforwardBy(105) // The split contract should expire after the fastforward

	// Simulate micropayment #1 between Alice and Bob, Carol should NOT get a cut
	payAmount := int64(10000)
	srcSeq, tgtSeq, paymentSeq, reserveSeq := 1, 1, 1, 1
	_ = createServicePaymentTx(et.chainID, &alice, &bob, 100, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	_ = createServicePaymentTx(et.chainID, &alice, &bob, 500, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	servicePaymentTx := createServicePaymentTx(et.chainID, &alice, &bob, payAmount, srcSeq, tgtSeq, paymentSeq, reserveSeq, resourceID)
	res = et.executor.getTxExecutor(servicePaymentTx).sanityCheck(et.chainID, et.state().Delivered(), servicePaymentTx)
	assert.True(res.IsOK(), res.Message)

	assert.Equal(0, len(et.state().GetSlashIntents()))
	_, res = et.executor.getTxExecutor(servicePaymentTx).process(et.chainID, et.state().Delivered(), servicePaymentTx)
	assert.True(res.IsOK(), res.Message)

	et.state().Commit()

	bobFinalBalance := et.state().GetAccount(bob.PubKey.Address()).Balance
	carolFinalBalance := et.state().GetAccount(carol.PubKey.Address()).Balance
	log.Infof("Bob's final balance:   %v", bobFinalBalance)
	log.Infof("Carol's final balance: %v", carolFinalBalance)

	// Check the balances of the relevant accounts
	bobSplitCoins := types.Coins{types.Coin{"GammaWei", int64(payAmount)}}
	servicePaymentTxFee := types.Coins{types.Coin{"GammaWei", 1}}
	assert.Equal(bobInitBalance.Plus(bobSplitCoins).Minus(servicePaymentTxFee), bobFinalBalance)
	assert.Equal(carolInitBalance, carolFinalBalance) // Carol gets no cut since the split contract has expired
}

func TestSplitContractTxUpdate(t *testing.T) {
	assert := assert.New(t)
	et, resourceID, _, _, carol, _, _, _ := setupForServicePayment(assert)
	et.fastforwardBy(1000)

	initiator := types.MakeAcc("User David")
	initiator.Balance = types.Coins{{"GammaWei", 10000 * 1e6}}
	et.acc2State(initiator)

	fakeInitiator := types.MakeAcc("User Eric")
	fakeInitiator.Balance = types.Coins{{"GammaWei", 10000 * 1e6}}
	et.acc2State(fakeInitiator)

	splitCarol := types.Split{
		Address:    carol.PubKey.Address(),
		Percentage: 30,
	}
	splitContractTx := &types.SplitContractTx{
		Fee:        types.Coin{"GammaWei", 10},
		ResourceID: resourceID,
		Initiator: types.TxInput{
			Address:  initiator.PubKey.Address(),
			PubKey:   initiator.PubKey,
			Sequence: 1,
		},
		Splits:   []types.Split{splitCarol},
		Duration: uint32(100),
	}
	signBytes := splitContractTx.SignBytes(et.chainID)
	splitContractTx.Initiator.Signature = initiator.Sign(signBytes)

	res := et.executor.getTxExecutor(splitContractTx).sanityCheck(et.chainID, et.state().Delivered(), splitContractTx)
	assert.True(res.IsOK(), res.Message)
	_, res = et.executor.getTxExecutor(splitContractTx).process(et.chainID, et.state().Delivered(), splitContractTx)
	assert.True(res.IsOK(), res.Message)

	splitContract := et.executor.state.GetSplitContract(resourceID)
	assert.NotNil(splitContract)
	originalEndHeight := splitContract.EndBlockHeight
	log.Infof("originalEndHeight = %v", originalEndHeight)

	// Another user tries to update the split contract, should fail
	fakeSplitContractUpdateTx := &types.SplitContractTx{
		Fee:        types.Coin{"GammaWei", 10},
		ResourceID: resourceID,
		Initiator: types.TxInput{
			Address:  fakeInitiator.PubKey.Address(),
			PubKey:   fakeInitiator.PubKey,
			Sequence: 1,
		},
		Splits:   []types.Split{splitCarol},
		Duration: uint32(1000),
	}
	signBytes = fakeSplitContractUpdateTx.SignBytes(et.chainID)
	fakeSplitContractUpdateTx.Initiator.Signature = fakeInitiator.Sign(signBytes)

	res = et.executor.getTxExecutor(fakeSplitContractUpdateTx).sanityCheck(et.chainID, et.state().Delivered(), fakeSplitContractUpdateTx)
	assert.False(res.IsOK(), res.Message)
	assert.Equal(result.CodeUnauthorizedToUpdateSplitContract, res.Code)
	_, res = et.executor.getTxExecutor(fakeSplitContractUpdateTx).process(et.chainID, et.state().Delivered(), fakeSplitContractUpdateTx)
	assert.False(res.IsOK(), res.Message)
	assert.Equal(result.CodeUnauthorizedToUpdateSplitContract, res.Code)

	splitContract1 := et.executor.state.GetSplitContract(resourceID)
	assert.NotNil(splitContract1)
	endHeight1 := splitContract1.EndBlockHeight
	assert.Equal(originalEndHeight, endHeight1)
	log.Infof("endHeight1 = %v", endHeight1)

	// The original initiator tries to update the split contract, should succeed
	extendedDuration := uint32(1000)
	splitContractUpdateTx := &types.SplitContractTx{
		Fee:        types.Coin{"GammaWei", 10},
		ResourceID: resourceID,
		Initiator: types.TxInput{
			Address:  initiator.PubKey.Address(),
			Sequence: 2,
		},
		Splits:   []types.Split{splitCarol},
		Duration: extendedDuration,
	}
	signBytes = splitContractUpdateTx.SignBytes(et.chainID)
	splitContractUpdateTx.Initiator.Signature = initiator.Sign(signBytes)

	res = et.executor.getTxExecutor(splitContractUpdateTx).sanityCheck(et.chainID, et.state().Delivered(), splitContractUpdateTx)
	assert.True(res.IsOK(), res.Message)
	_, res = et.executor.getTxExecutor(splitContractUpdateTx).process(et.chainID, et.state().Delivered(), splitContractUpdateTx)
	assert.True(res.IsOK(), res.Message)

	splitContract2 := et.executor.state.GetSplitContract(resourceID)
	assert.NotNil(splitContract2)
	currHeight := et.executor.state.Height()
	endHeight2 := splitContract2.EndBlockHeight
	assert.Equal(currHeight+extendedDuration, endHeight2)
	log.Infof("currHeight = %v", currHeight)
	log.Infof("endHeight2 = %v", endHeight2)
}

// ------------------------------ Test Utils ------------------------------ //

func createServicePaymentTx(chainID string, source, target *types.PrivAccount, amount int64, srcSeq, tgtSeq, paymentSeq, reserveSeq int, resourceID common.Bytes) *types.ServicePaymentTx {
	servicePaymentTx := &types.ServicePaymentTx{
		Fee: types.Coin{"GammaWei", 1},
		Source: types.TxInput{
			Address:  source.PubKey.Address(),
			Coins:    types.Coins{{"GammaWei", amount}},
			Sequence: srcSeq,
		},
		Target: types.TxInput{
			Address:  target.PubKey.Address(),
			Sequence: tgtSeq,
		},
		PaymentSequence: paymentSeq,
		ReserveSequence: reserveSeq,
		ResourceID:      resourceID,
	}

	if srcSeq == 1 {
		servicePaymentTx.Source.PubKey = source.PubKey
	}
	if tgtSeq == 1 {
		servicePaymentTx.Target.PubKey = target.PubKey
	}

	srcSignBytes := servicePaymentTx.SourceSignBytes(chainID)
	servicePaymentTx.Source.Signature = source.Sign(srcSignBytes)

	tgtSignBytes := servicePaymentTx.TargetSignBytes(chainID)
	servicePaymentTx.Target.Signature = target.Sign(tgtSignBytes)

	if !source.PubKey.VerifySignature(srcSignBytes, servicePaymentTx.Source.Signature) {
		panic("Signature verification failed for source")
	}
	if !target.PubKey.VerifySignature(tgtSignBytes, servicePaymentTx.Target.Signature) {
		panic("Signature verification failed for target")
	}

	return servicePaymentTx
}

func setupForServicePayment(ast *assert.Assertions) (et *execTest, resourceID common.Bytes,
	alice, bob, carol types.PrivAccount, aliceInitBalance, bobInitBalance, carolInitBalance types.Coins) {
	et = newExecTest()

	alice = types.MakeAcc("User Alice")
	aliceInitBalance = types.Coins{{"GammaWei", 10000 * 1e6}}
	alice.Balance = aliceInitBalance
	et.acc2State(alice)
	log.Infof("Alice's pubKey: %v", hex.EncodeToString(alice.PubKey.ToBytes()))
	log.Infof("Alice's Address: %v", alice.PubKey.Address().Hex())

	bob = types.MakeAcc("User Bob")
	bobInitBalance = types.Coins{{"GammaWei", 3000 * 1e6}}
	bob.Balance = bobInitBalance
	et.acc2State(bob)
	log.Infof("Bob's pubKey:   %v", hex.EncodeToString(bob.PubKey.ToBytes()))
	log.Infof("Bob's Address: %v", bob.PubKey.Address().Hex())

	carol = types.MakeAcc("User Carol")
	carolInitBalance = types.Coins{{"GammaWei", 3000 * 1e6}}
	carol.Balance = carolInitBalance
	et.acc2State(carol)
	log.Infof("Carol's pubKey: %v", hex.EncodeToString(carol.PubKey.ToBytes()))
	log.Infof("Carol's Address: %v", carol.PubKey.Address().Hex())

	et.fastforwardTo(1e2)

	resourceID = common.Bytes("rid001")
	reserveFundTx := &types.ReserveFundTx{
		Fee: types.Coin{"GammaWei", 100},
		Source: types.TxInput{
			Address:  alice.PubKey.Address(),
			PubKey:   alice.PubKey,
			Coins:    types.Coins{{"GammaWei", 1000 * 1e6}},
			Sequence: 1,
		},
		Collateral:  types.Coins{{"GammaWei", 1001 * 1e6}},
		ResourceIDs: []common.Bytes{resourceID},
		Duration:    1000,
	}
	reserveFundTx.Source.Signature = alice.Sign(reserveFundTx.SignBytes(et.chainID))
	res := et.executor.getTxExecutor(reserveFundTx).sanityCheck(et.chainID, et.state().Delivered(), reserveFundTx)
	ast.True(res.IsOK(), res.String())
	_, res = et.executor.getTxExecutor(reserveFundTx).process(et.chainID, et.state().Delivered(), reserveFundTx)
	ast.True(res.IsOK(), res.String())

	return et, resourceID, alice, bob, carol, aliceInitBalance, bobInitBalance, carolInitBalance
}
