// SPDX-License-Identifier: GPL-3.0-or-later
#include <jni.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include "_cgo_export.h"
char *ds_request(uintptr_t e, uintptr_t activity, const char *action, const char *payload){
 JNIEnv *env=(JNIEnv*)e; jobject ctx=(jobject)activity; jclass cls=(*env)->GetObjectClass(env,ctx);
 jmethodID method=(*env)->GetMethodID(env,cls,"bridgeRequest","(Ljava/lang/String;[B)[B");
 if(!method){(*env)->DeleteLocalRef(env,cls);return NULL;}
 jstring a=(*env)->NewStringUTF(env,action);
 jsize length=(jsize)strlen(payload);
 jbyteArray p=(*env)->NewByteArray(env,length);
 (*env)->SetByteArrayRegion(env,p,0,length,(const jbyte*)payload);
 jbyteArray result=(jbyteArray)(*env)->CallObjectMethod(env,ctx,method,a,p);char *copy=NULL;
 if(!(*env)->ExceptionCheck(env)&&result){
  jsize n=(*env)->GetArrayLength(env,result);copy=malloc((size_t)n+1);
  if(copy){(*env)->GetByteArrayRegion(env,result,0,n,(jbyte*)copy);copy[n]='\0';}
  (*env)->DeleteLocalRef(env,result);
 }
 (*env)->DeleteLocalRef(env,a);(*env)->DeleteLocalRef(env,p);(*env)->DeleteLocalRef(env,cls);return copy;
}
void ds_launch(uintptr_t e,uintptr_t activity,const char *action){JNIEnv *env=(JNIEnv*)e;jobject ctx=(jobject)activity;jclass cls=(*env)->GetObjectClass(env,ctx);jmethodID method=(*env)->GetMethodID(env,cls,"bridgeLaunch","(Ljava/lang/String;)V");if(method){jstring a=(*env)->NewStringUTF(env,action);(*env)->CallVoidMethod(env,ctx,method,a);(*env)->DeleteLocalRef(env,a);}(*env)->DeleteLocalRef(env,cls);}
JNIEXPORT void JNICALL Java_ca_cerberusgames_douchesync_FyneActivity_uiChanged(JNIEnv *env,jobject activity){ds_ui_changed();}
