/*
 * Tencent is pleased to support the open source community by making 蓝鲸 available.
 * Copyright (C) 2017-2018 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package logics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"configcenter/src/common"
	"configcenter/src/common/blog"
	"configcenter/src/common/metadata"
	"configcenter/src/common/util"
)

func getMustNeedHeader(header http.Header) http.Header {
	mustNeedHeader := make(http.Header, 0)
	mustNeedHeader.Set(common.BKHTTPOwnerID, util.GetOwnerID(header))
	mustNeedHeader.Set(common.BKHTTPLanguage, util.GetLanguage(header))
	mustNeedHeader.Set(common.BKHTTPHeaderUser, util.GetUser(header))
	mustNeedHeader.Set(common.BKHTTPCCRequestID, util.GetHTTPCCRequestID(header))
	return mustNeedHeader
}

func getEventRefrshModuleKey(appID, moduleID int64) string {
	return fmt.Sprintf("%d-%d", appID, moduleID)
}

func (lgc *Logics) addEventRefreshModuleItem(appID, moduleID int64) error {

	info := refreshHostInstModuleID{AppID: appID, ModuleID: moduleID, Header: getMustNeedHeader(lgc.header)}
	valInfo, err := json.Marshal(info)
	if nil != err {
		blog.Warnf("addEventRefreshModuleItem appID:%d, moduleID:%d, error:%s,rid:%s", appID, moduleID, err.Error(), lgc.rid)
		return nil
	}
	err = lgc.cache.SAdd(common.RedisProcSrvHostInstanceRefreshModuleKey, string(valInfo)).Err()
	if nil != err {
		blog.Errorf("addEventRefreshModuleItem save info to cache appID:%d, infos:%v, redis sadd error:%s,rid:%s", appID, valInfo, err.Error(), lgc.rid)
		return lgc.ccErr.Errorf(common.CCErrCommUtilHandleFail, "redis sadd", err.Error())
	}
	return nil
}

func (lgc *Logics) addEventRefreshModuleItems(ctx context.Context, appID int64, moduleIDs []int64) error {
	mustNeedHeader := getMustNeedHeader(lgc.header)
	valInfoArrs := make([]interface{}, 0)
	for _, moduleID := range moduleIDs {
		info := refreshHostInstModuleID{AppID: appID, ModuleID: moduleID, Header: mustNeedHeader}
		valInfo, err := json.Marshal(info)
		if nil != err {
			blog.Warnf("addEventRefreshModuleItem appID:%d, moduleID:%d,json marshal error:%s,rid:%s", appID, moduleID, err.Error(), lgc.rid)
			continue
		}
		valInfoArrs = append(valInfoArrs, string(valInfo))
	}
	err := lgc.cache.SAdd(common.RedisProcSrvHostInstanceRefreshModuleKey, valInfoArrs...).Err()
	if nil != err {
		blog.Errorf("addEventRefreshModuleItem save info to cache appID:%d, infos:%v, redis sadd error:%s,rid:%s", appID, valInfoArrs, err.Error(), lgc.rid)
		return lgc.ccErr.Errorf(common.CCErrCommUtilHandleFail, "redis sadd", err.Error())
	}
	return nil
}

func (lgc *Logics) unregisterProcInstDetall(ctx context.Context, appID, moduleID int64) error {
	defErr := lgc.ccErr
	dat := new(metadata.QueryInput)
	dat.Condition = map[string]interface{}{common.BKAppIDField: appID, common.BKModuleIDField: moduleID, common.BKStatusField: metadata.ProcInstanceDetailStatusUnRegisterFailed}
	dat.Limit = 200
	dat.Start = 0
	for {
		ret, err := lgc.CoreAPI.ProcController().GetProcInstanceDetail(ctx, lgc.header, dat)
		if nil != err {
			blog.Errorf("unregisterProcInstDetall GetProcInstanceDetail http do error. proc instance error:%s,input:%+v,rid:%s", err.Error(), dat, lgc.rid)
			return defErr.Error(common.CCErrCommHTTPDoRequestFailed)
		}
		if !ret.Result {
			blog.Errorf("unregisterProcInstDetall  GetProcInstanceDetail http reply error. proc instance return err code:%d, err msg %s,input:%+v,rid:%s", ret.Code, ret.ErrMsg, dat, lgc.rid)
			return defErr.New(ret.Code, ret.ErrMsg)
		}
		if 0 == ret.Data.Count {
			return nil
		}
		for _, item := range ret.Data.Info {
			if 0 == len(item.Hosts) {
				continue
			}
			gseProc := new(metadata.GseProcRequest)
			gseProc.OpType = common.GSEProcOPUnregister
			gseProc.AppID = item.AppID
			gseProc.ProcID = item.ProcID
			gseProc.Hosts = item.Hosts
			gseProc.Meta = item.Meta
			gseProc.ModuleID = item.ModuleID
			gseProc.Spec = item.Spec
			err := lgc.unregisterProcInstanceToGse(ctx, gseProc)
			if nil != err {
				return err
			}
		}
	}

	return nil

}

// setProcInstDetallStatusUnregister modify process instance status to unregister in cmdb table
func (lgc *Logics) setProcInstDetallStatusUnregister(ctx context.Context, appID, moduleID int64) error {

	defErr := lgc.ccErr

	dat := new(metadata.ModifyProcInstanceDetail)
	dat.Conds = map[string]interface{}{common.BKAppIDField: appID, common.BKModuleIDField: moduleID}
	dat.Data = map[string]interface{}{common.BKStatusField: metadata.ProcInstanceDetailStatusUnRegisterFailed}
	ret, err := lgc.CoreAPI.ProcController().ModifyProcInstanceDetail(ctx, lgc.header, dat)
	if nil != err {
		blog.Errorf("setProcInstDetallStatusUnregister ModifyProcInstanceDetail http do error. proc instance error:%s,input:%+v,rid:%s", err.Error(), dat, lgc.rid)
		return defErr.Error(common.CCErrCommHTTPDoRequestFailed)
	}
	if !ret.Result {
		blog.Errorf("setProcInstDetallStatusUnregister  ModifyProcInstanceDetail http reply error. proc instance return err code:%s, err msg:%s,input:%+v,rid:%s", ret.Code, ret.ErrMsg, dat, lgc.rid)
		return defErr.New(ret.Code, ret.ErrMsg)
	}

	return nil
}

func (lgc *Logics) handleProcInstNumDataHandle(ctx context.Context, appID, moduleID int64, instProc []*metadata.ProcInstanceModel) error {
	delConds := common.KvMap{common.BKAppIDField: appID, common.BKModuleIDField: moduleID}
	defErr := lgc.ccErr
	ret, err := lgc.CoreAPI.ProcController().DeleteProcInstanceModel(ctx, lgc.header, delConds)
	if nil != err {
		blog.Errorf("handleInstanceNum DeleteProcInstanceModel http do error. error:%s, input:%+v,rid:%s", err.Error(), delConds, lgc.rid)
		return defErr.Error(common.CCErrCommHTTPDoRequestFailed)
	}
	if !ret.Result {
		blog.Errorf("handleInstanceNum  DeleteProcInstanceModel http reply error. err code:%d,err msg:%s,input:%+v,rid:%s", ret.Code, ret.ErrMsg, delConds, lgc.rid)
		return defErr.New(ret.Code, ret.ErrMsg)
	}
	if 0 < len(instProc) {
		ret, err := lgc.CoreAPI.ProcController().CreateProcInstanceModel(ctx, lgc.header, instProc)
		if nil != err {
			blog.Errorf("handleInstanceNum CreateProcInstanceModel http do error. create proc instance error:%s,input:%+v,rid:%s", err.Error(), instProc, lgc.rid)
			return defErr.Error(common.CCErrCommHTTPDoRequestFailed)
		}
		if !ret.Result {
			blog.Errorf("handleInstanceNum  CreateProcInstanceModel http do error. create proc instance err code:%d, err msg:%s,input:%+v,rid:%s", ret.Code, ret.ErrMsg, instProc, lgc.rid)
			return defErr.New(ret.Code, ret.ErrMsg)
		}
	}

	return nil
}

func (lgc *Logics) getProcInstInfoByModuleID(ctx context.Context, appID, moduleID int64) (maxHostInstID uint64, procInst map[string]metadata.ProcInstanceModel, err error) {
	dat := new(metadata.QueryInput)
	dat.Condition = common.KvMap{common.BKModuleIDField: moduleID, common.BKAppIDField: appID}
	dat.Limit = common.BKNoLimit
	defErr := lgc.ccErr
	ret, err := lgc.CoreAPI.ProcController().GetProcInstanceModel(ctx, lgc.header, dat)
	if nil != err {
		blog.Errorf("getProcInstInfoByModuleID http do error, error:%s,input:%+v,rid:%s", err.Error(), dat, lgc.rid)
		return 0, nil, defErr.Error(common.CCErrCommHTTPDoRequestFailed)
	}
	if !ret.Result {
		blog.Errorf("getProcInstInfoByModuleID  htpp reply error. err code:%d,err msg:%s,input:%+v,rid:%s", ret.Code, ret.ErrMsg, dat, lgc.rid)
		return 0, nil, defErr.New(ret.Code, ret.ErrMsg)
	}
	procInst = make(map[string]metadata.ProcInstanceModel, 0)
	for _, item := range ret.Data.Info {
		if maxHostInstID < item.HostInstanID {
			maxHostInstID = item.HostInstanID
		}
		procInst[getInlineProcInstKey(int64(item.HostID), int64(item.ModuleID))] = item
	}

	return maxHostInstID, procInst, nil

}

func (lgc *Logics) getModuleBindProc(ctx context.Context, appID, moduleID int64, header http.Header) (setID int64, procID []int64, err error) {
	supplierID := lgc.ownerID
	var name string
	defErr := lgc.CCErr.CreateDefaultCCErrorIf(util.GetLanguage(header))
	name, appID, setID, err = lgc.getModuleNameByID(ctx, moduleID)
	if nil != err {
		blog.Errorf("getModuleBindProc error:%s,rid:%s", err.Error(), lgc.rid)
		return 0, nil, err
	}
	dat := common.KvMap{common.BKModuleNameField: name, common.BKAppIDField: appID}
	ret, err := lgc.CoreAPI.ProcController().GetProc2Module(ctx, header, dat)
	if nil != err {
		blog.Errorf("getModuleBindProc GetProc2Module http do error. moduleID %d supplierID %s  error:%s,input:%+v,,rid:%s", moduleID, supplierID, err.Error())
		return 0, nil, defErr.Error(common.CCErrCommHTTPDoRequestFailed)
	}
	if !ret.Result {
		blog.Errorf("getModuleBindProc GetProc2Module http reply error. moduleID %d supplierID %s, err code:%d, err msg:%s,input:%+v,rid:%d", moduleID, supplierID, ret.Code, ret.ErrMsg, dat, lgc.rid)
		return 0, nil, defErr.New(ret.Code, ret.ErrMsg)
	}
	for _, proc := range ret.Data {
		procID = append(procID, proc.ProcessID)
	}

	return setID, procID, nil
}
