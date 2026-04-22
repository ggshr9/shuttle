package com.shuttle.app

import androidx.test.ext.junit.rules.ActivityScenarioRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.LargeTest
import org.junit.Assert.assertFalse
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Android launch smoke test.
 *
 * Minimal baseline — MainActivity starts without crashing. Asserting on
 * WebView content requires Espresso's web-view matchers, which depend on
 * the Svelte build being present in `assets/web/` (only true after a full
 * build-android.sh run). Left for a follow-up once the CI run is
 * consistently producing APKs.
 */
@LargeTest
@RunWith(AndroidJUnit4::class)
class MainActivityTest {

    @get:Rule
    val rule = ActivityScenarioRule(MainActivity::class.java)

    @Test
    fun activityStartsWithoutImmediateCrash() {
        rule.scenario.onActivity { activity ->
            assertFalse(
                "MainActivity finished immediately after launch",
                activity.isFinishing
            )
        }
    }
}
