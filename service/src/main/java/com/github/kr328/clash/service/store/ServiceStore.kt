package com.github.kr328.clash.service.store

import android.content.Context
import com.github.kr328.clash.common.store.Store
import com.github.kr328.clash.common.store.asStoreProvider
import com.github.kr328.clash.service.PreferenceProvider
import com.github.kr328.clash.service.model.AccessControlMode
import java.util.*

class ServiceStore(context: Context) {
    private val store = Store(
        PreferenceProvider
            .createSharedPreferencesFromContext(context)
            .asStoreProvider()
    )

    var activeProfile: UUID? by store.typedString(
        key = "active_profile",
        from = { if (it.isBlank()) null else UUID.fromString(it) },
        to = { it?.toString() ?: "" }
    )

    var bypassPrivateNetwork: Boolean by store.boolean(
        key = "bypass_private_network",
        defaultValue = true
    )

    var accessControlMode: AccessControlMode by store.enum(
        key = "access_control_mode",
        defaultValue = AccessControlMode.DenySelected,
        values = AccessControlMode.values()
    )

    var accessControlPackages by store.stringSet(
        key = "access_control_packages",
        defaultValue = DEFAULT_ACCESS_CONTROL_PACKAGES
    )

    var dnsHijacking by store.boolean(
        key = "dns_hijacking",
        defaultValue = true
    )

    var systemProxy by store.boolean(
        key = "system_proxy",
        defaultValue = true
    )

    var allowBypass by store.boolean(
        key = "allow_bypass",
        defaultValue = true
    )

    var allowIpv6 by store.boolean(
        key = "allow_ipv6",
        defaultValue = false
    )

    var tunStackMode by store.string(
        key = "tun_stack_mode",
        defaultValue = "mips"
    )

    var dynamicNotification by store.boolean(
        key = "dynamic_notification",
        defaultValue = true
    )

    companion object {
        // Apps that should connect directly (bypass the VPN) unless the user changes the list
        private val DEFAULT_ACCESS_CONTROL_PACKAGES = setOf(
            "com.ss.android.ugc.aweme",
            "cn.samsclub.app",
            "com.tencent.wework",
            "com.unionpay",
            "com.tencent.mm",
            "com.manmanbuy.bijia",
            "com.hwabao.hbstockwarning",
            "com.eastmoney.android.berlin",
            "com.tencent.weread",
            "com.tmri.app.main",
            "com.szlanyou.nissaniov",
            "com.hexin.plat.android",
            "com.cmbc.cc.mbank",
            "com.nocode.nutridecode",
            "cn.com.cmbc.newmbank",
            "com.citiccard.mobilebank",
            "cn.gov.tax.its",
            "com.chinamworld.main",
            "com.chinamworld.bocmbci",
            "com.cloudpower.netsale.activity",
            "com.ss.android.ugc.lifeservices",
            "cn.yingmi.qieman.hermione",
            "cn.com.hzb.mobilebank.per",
            "com.tencent.wetype",
            "com.android.bankabc",
            "cmb.pb",
            "tv.danmaku.bili",
            "com.blizzard.wtcg.hearthstone",
            "com.smzdm.client.android",
            "com.jingdong.app.mall",
        )
    }
}