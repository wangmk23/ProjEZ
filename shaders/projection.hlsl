cbuffer Settings : register(b0) {
 float4 source; // logical width, height, DXGI rotation, pointer type
 float4 cursorRect;
 float4 options; // cursor enabled/visible, scale detail strength
};
Texture2D desktop : register(t0);
Texture2D pointerImage : register(t1);
SamplerState sampleMode : register(s0);
struct Vertex { float4 position : SV_POSITION; float2 uv : TEXCOORD0; };
Vertex VS(uint id : SV_VertexID) {
 Vertex v;
 v.uv = float2((id << 1) & 2, id & 2);
 v.position = float4(v.uv * float2(2,-2) + float2(-1,1),0,1);
 return v;
}
// Integrate covered source pixels instead of sampling only the output center.
// Bounded work for large reductions; the usual 1080p/1440p reductions use
// exact overlap weights and retain thin strokes between sample centers.
float4 reducedDesktop(float2 uv, float2 footprint, float2 dimensions) {
 float2 center=uv*dimensions;
 float2 low=center-footprint*0.5, high=center+footprint*0.5;
 float3 sum=float3(0,0,0);
 float total=0;
 if(all(footprint<=7)) {
  int2 first=(int2)floor(low);
  int2 count=(int2)ceil(high)-first;
  [loop] for(int y=0;y<count.y;y++) {
   [loop] for(int x=0;x<count.x;x++) {
    int2 at=first+int2(x,y);
    float2 overlap=max(0,min(high,float2(at+1))-max(low,float2(at)));
    float weight=overlap.x*overlap.y;
    int2 clamped=clamp(at,int2(0,0),(int2)dimensions-1);
    sum+=desktop.Load(int3(clamped,0)).rgb*weight;
    total+=weight;
   }
  }
 } else {
 [loop] for(int j=0;j<8;j++) {
  [loop] for(int i=0;i<8;i++) {
   float2 at=(low+footprint*(float2(i,j)+0.5)/8)/dimensions;
   sum+=desktop.SampleLevel(sampleMode,at,0).rgb;
  }
 }
 total=64;
 }
 return float4(sum/max(total,0.0001),1);
}
float4 PS(Vertex v) : SV_TARGET {
 float2 uv=v.uv;
 if(source.z==2) uv=float2(v.uv.y,1-v.uv.x);
 if(source.z==3) uv=1-v.uv;
 if(source.z==4) uv=float2(1-v.uv.y,v.uv.x);
 float4 bg=desktop.Sample(sampleMode,uv);bg.a=1;
 if(options.w>0) {
  uint width,height; desktop.GetDimensions(width,height);
  float2 dimensions=float2(width,height);
  float2 footprint=max((abs(ddx(uv))+abs(ddy(uv)))*dimensions,1);
  bg=reducedDesktop(uv,footprint,dimensions);
 }
 // Mild single-pass detail recovery for scaled content only. Clamp to the
 // neighborhood range so text edges cannot introduce bright/dark halos.
 if(options.z>0) {
  uint tw,th; desktop.GetDimensions(tw,th);
  float2 step=1.0/float2(tw,th);
  float3 l=desktop.SampleLevel(sampleMode,uv-float2(step.x,0),0).rgb;
  float3 r=desktop.SampleLevel(sampleMode,uv+float2(step.x,0),0).rgb;
  float3 t=desktop.SampleLevel(sampleMode,uv-float2(0,step.y),0).rgb;
  float3 b=desktop.SampleLevel(sampleMode,uv+float2(0,step.y),0).rgb;
  float3 lo=min(bg.rgb,min(min(l,r),min(t,b)));
  float3 hi=max(bg.rgb,max(max(l,r),max(t,b)));
  bg.rgb=clamp(bg.rgb+options.z*(bg.rgb-(l+r+t+b)*0.25),lo,hi);
 }
 float2 p=floor(v.uv*source.xy)-cursorRect.xy;
 if(options.x>0 && all(p>=0) && all(p<cursorRect.zw)) {
  float4 c=pointerImage.Load(int3((int2)p,0));
  if(source.w==1) {
   uint3 bits=(uint3)round(saturate(bg.rgb)*255);
   bits=(bits & (c.r>0.5 ? 255u : 0u)) ^ (c.g>0.5 ? 255u : 0u);
   bg.rgb=float3(bits)/255;
  } else if(source.w==4) {
   uint3 bits=(uint3)round(saturate(bg.rgb)*255);
   uint3 mask=(uint3)round(c.rgb*255);
   bg.rgb=c.a>0.5 ? float3(bits ^ mask)/255 : c.rgb;
  } else { bg.rgb=c.rgb*c.a+bg.rgb*(1-c.a); }
 }
 return bg;
}
